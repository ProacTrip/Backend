package callback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Mock types para oauth/callback UseCase tests
// =============================================================================

// mockStateTokenSvc simula OAuthStateTokenService.
type mockStateTokenSvc struct {
	stateValue string
	err        error
}

func (m *mockStateTokenSvc) ValidateOAuthStateToken(tokenString string) (string, error) {
	return m.stateValue, m.err
}

// mockOAuthProvider implementa domain.OAuthProvider.
type mockOAuthProvider struct {
	exchangeErr error
	userInfo    *domain.OAuthUserInfo
	userInfoErr error
	authURL     string
}

func (m *mockOAuthProvider) GetAuthURL(state, codeChallenge string) string { return m.authURL }
func (m *mockOAuthProvider) ExchangeCode(ctx context.Context, code, codeVerifier string) (*domain.OAuthToken, error) {
	if m.exchangeErr != nil {
		return nil, m.exchangeErr
	}
	return &domain.OAuthToken{
		AccessToken:  "mock-access-token",
		RefreshToken: "mock-refresh-token",
		ExpiresIn:    3600,
		TokenType:    "Bearer",
	}, nil
}
func (m *mockOAuthProvider) GetUserInfo(ctx context.Context, accessToken string) (*domain.OAuthUserInfo, error) {
	if m.userInfoErr != nil {
		return nil, m.userInfoErr
	}
	return m.userInfo, nil
}

// mockProviderSel simula OAuthProviderSelector.
type mockProviderSel struct {
	provider domain.OAuthProvider
	err      error
}

func (m *mockProviderSel) GetProvider(providerCode string) (domain.OAuthProvider, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.provider, nil
}

// mockUserRepo simula domain.UserRepository.
type mockUserRepo struct {
	users     map[string]*domain.User
	roles     map[string]*domain.Role
	getByErr  error
	createErr error
	roleErr   error
	updateErr error

	createCalls []*domain.User
	updateCalls []*domain.User
}

func newMockUserRepo() *mockUserRepo {
	roleID := uuid.Must(uuid.NewV7())
	return &mockUserRepo{
		users: make(map[string]*domain.User),
		roles: map[string]*domain.Role{
			"client": {ID: roleID, Name: "client"},
		},
	}
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.getByErr != nil {
		return nil, m.getByErr
	}
	u, ok := m.users[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.createCalls = append(m.createCalls, user)
	m.users[user.Email] = user
	return nil
}

func (m *mockUserRepo) Update(ctx context.Context, user *domain.User) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updateCalls = append(m.updateCalls, user)
	return nil
}

func (m *mockUserRepo) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	if m.roleErr != nil {
		return nil, m.roleErr
	}
	r, ok := m.roles[name]
	if !ok {
		return nil, domain.ErrRoleNotFound
	}
	return r, nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) { return nil, nil }

// mockOAuthRepo simula domain.OAuthRepository.
type mockOAuthRepo struct {
	identities       map[string]*domain.AuthIdentity // key: "providerCode:providerUserID"
	getByIdentityErr error
	createErr        error
	updateErr        error

	createCalls  []*domain.AuthIdentity
	updateCalls  []*domain.AuthIdentity
}

func newMockOAuthRepo() *mockOAuthRepo {
	return &mockOAuthRepo{
		identities: make(map[string]*domain.AuthIdentity),
	}
}

func (m *mockOAuthRepo) CreateAuthIdentity(ctx context.Context, identity *domain.AuthIdentity) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.createCalls = append(m.createCalls, identity)
	key := identity.ProviderCode + ":" + identity.ProviderUserID
	m.identities[key] = identity
	return nil
}

func (m *mockOAuthRepo) GetAuthIdentityByProvider(ctx context.Context, providerCode, providerUserID string) (*domain.AuthIdentity, error) {
	if m.getByIdentityErr != nil {
		return nil, m.getByIdentityErr
	}
	key := providerCode + ":" + providerUserID
	id, ok := m.identities[key]
	if !ok {
		return nil, domain.ErrIdentityNotFound
	}
	return id, nil
}

func (m *mockOAuthRepo) GetAuthIdentitiesByUser(ctx context.Context, userID uuid.UUID) ([]*domain.AuthIdentity, error) {
	return nil, nil
}

func (m *mockOAuthRepo) UpdateAuthIdentity(ctx context.Context, identity *domain.AuthIdentity) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updateCalls = append(m.updateCalls, identity)
	return nil
}

// mockTokenSvc simula TokenService.
type mockTokenSvc struct {
	pair *token.TokenPair
	err  error
}

func (m *mockTokenSvc) GenerateTokenPair(userID uuid.UUID, email string, role string, roleID uuid.UUID) (*token.TokenPair, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pair, nil
}

// mockEventPublisher simula EventPublisher.
type mockEventPublisher struct {
	published []publishedEvent
}

type publishedEvent struct {
	stream  string
	payload map[string]any
}

func (m *mockEventPublisher) Publish(ctx context.Context, stream string, payload map[string]any) (string, error) {
	m.published = append(m.published, publishedEvent{stream: stream, payload: payload})
	return "msg-1", nil
}

// =============================================================================
// Factory — crea una UseCase lista para pruebas con miniredis y mocks.
// =============================================================================

func newTestUseCase(t *testing.T) (*UseCase, *miniredis.Miniredis, *mockUserRepo, *mockOAuthRepo, *mockTokenSvc, *mockEventPublisher) {
	t.Helper()

	mr := miniredis.RunT(t)
	dragonfly := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	repo := newMockUserRepo()
	oauthRepo := newMockOAuthRepo()
	tokenSvc := &mockTokenSvc{
		pair: &token.TokenPair{
			AccessToken:  "access-token-mock",
			RefreshToken: "refresh-token-mock",
		},
	}
	eventPub := &mockEventPublisher{}

	provider := &mockOAuthProvider{
		userInfo: &domain.OAuthUserInfo{
			ProviderUserID: "google-sub-123",
			Email:          "test@proactrip.com",
			EmailVerified:  true,
			Name:           "Test User",
			GivenName:      "Test",
			FamilyName:     "User",
			Locale:         "es-419",
			Picture:        "https://example.com/avatar.png",
		},
	}
	providerSel := &mockProviderSel{provider: provider}

	stateTokenSvc := &mockStateTokenSvc{stateValue: "state-abc-123"}

	uc := &UseCase{
		repo:           repo,
		oauthRepo:      oauthRepo,
		stateTokenSvc:  stateTokenSvc,
		providerSel:    providerSel,
		tokenSvc:       tokenSvc,
		dragonfly:      dragonfly,
		eventPublisher: eventPub,
	}

	return uc, mr, repo, oauthRepo, tokenSvc, eventPub
}

// helper: poblar el estado OAuth en miniredis
func populateOAuthState(miniredis *miniredis.Miniredis, stateValue, codeVerifier string) {
	state := domain.OAuthState{
		CodeVerifier: codeVerifier,
	}
	data, _ := json.Marshal(state)
	key := "{auth}:oauth:state:" + stateValue
	miniredis.Set(key, string(data))
}

// helper: crear comando de callback válido
func validCommand() Command {
	return Command{
		ProviderCode: "auth-code-from-google",
		State:        "paseto-state-token",
		Provider:     "google",
	}
}

// helper: verificar que un usuario ya existe en el mock repo
func seedExistingUser(repo *mockUserRepo, email string) *domain.User {
	roleID := uuid.Must(uuid.NewV7())
	user := &domain.User{
		ID:             uuid.Must(uuid.NewV7()),
		Email:          email,
		EmailVerified:  true,
		PasswordHash:   "",
		RoleID:         roleID,
		RoleName:       "client",
		Status:         domain.StatusActive,
		LoginCount:     0,
		FailedLoginAttempts: 0,
	}
	repo.users[email] = user
	return user
}

// =============================================================================
// Triangulación: OAuth sin FamilyName ni Locale → NO aparecen en el evento.
// =============================================================================

func TestExecute_NuevoUsuario_SinFamilyNameNiLocale_NoAparecenEnPayload(t *testing.T) {
	uc, mr, _, _, _, eventPub := newTestUseCase(t)
	ctx := t.Context()

	// Pre-poblar Dragonfly con el estado OAuth
	populateOAuthState(mr, "state-abc-123", "code-verifier-xyz")

	// Sobrescribir el userInfo del mock para que no tenga FamilyName ni Locale
	uc.providerSel.(*mockProviderSel).provider.(*mockOAuthProvider).userInfo.FamilyName = ""
	uc.providerSel.(*mockProviderSel).provider.(*mockOAuthProvider).userInfo.Locale = ""

	cmd := validCommand()
	_, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	if len(eventPub.published) != 1 {
		t.Fatalf("eventPublisher.Publish fue llamado %d veces, want 1", len(eventPub.published))
	}
	payload := eventPub.published[0].payload

	if _, ok := payload["last_name"]; ok {
		t.Error("event payload NO debe incluir last_name cuando FamilyName está vacío")
	}
	if _, ok := payload["locale"]; ok {
		t.Error("event payload NO debe incluir locale cuando Locale está vacío")
	}
}

// =============================================================================
// Escenario 1: state_valido — flujo completo exitoso con usuario existente.
// =============================================================================

func TestExecute_EstadoValido_UsuarioExistente_RetornaTokens(t *testing.T) {
	uc, mr, repo, oauthRepo, _, _ := newTestUseCase(t)
	ctx := t.Context()

	// Pre-poblar Dragonfly con el estado OAuth
	populateOAuthState(mr, "state-abc-123", "code-verifier-xyz")

	// Pre-poblar usuario existente
	existingUser := seedExistingUser(repo, "test@proactrip.com")

	// Ejecutar
	cmd := validCommand()
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	// Verificar respuesta
	if resp.AccessToken != "access-token-mock" {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "access-token-mock")
	}
	if resp.RefreshToken != "refresh-token-mock" {
		t.Errorf("RefreshToken = %q, want %q", resp.RefreshToken, "refresh-token-mock")
	}
	if resp.User == nil {
		t.Fatal("User response es nil")
	}
	if resp.User.UserID != existingUser.ID {
		t.Errorf("UserID = %v, want %v", resp.User.UserID, existingUser.ID)
	}
	if resp.User.Email != "test@proactrip.com" {
		t.Errorf("Email = %q, want %q", resp.User.Email, "test@proactrip.com")
	}

	// Verificar que NO se creó un nuevo usuario
	if len(repo.createCalls) > 0 {
		t.Errorf("repo.Create fue llamado %d veces, se esperaba 0 (usuario ya existía)", len(repo.createCalls))
	}

	// Verificar que el login fue registrado
	if len(repo.updateCalls) == 0 {
		t.Error("repo.Update no fue llamado — RecordLogin debería haberse ejecutado")
	} else {
		updatedUser := repo.updateCalls[0]
		if updatedUser.LoginCount != 1 {
			t.Errorf("LoginCount = %d, want 1", updatedUser.LoginCount)
		}
	}

	// Verificar que la identidad fue registrada (nueva identidad para el usuario existente)
	if len(oauthRepo.createCalls) != 1 {
		t.Errorf("oauthRepo.CreateAuthIdentity fue llamado %d veces, want 1", len(oauthRepo.createCalls))
	}
}

// =============================================================================
// Escenario 8: stateTokenSvc falla → error inmediato sin tocar Dragonfly.
// =============================================================================

func TestExecute_EstadoInvalido_StateNoEnCache_DevuelveErrOAuthStateInvalid(t *testing.T) {
	uc, _, _, _, _, _ := newTestUseCase(t)
	ctx := t.Context()

	// NO pre-poblar Dragonfly → el GET devuelve redis.Nil

	cmd := validCommand()
	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("Execute: se esperaba error, pero fue nil")
	}
	if !errors.Is(err, domain.ErrOAuthStateInvalid) {
		t.Errorf("error = %v, want %v", err, domain.ErrOAuthStateInvalid)
	}
}

// =============================================================================
// Escenario 3: dragonfly_get_del — verificar eliminación atómica GET+DEL.
// =============================================================================

func TestExecute_EstadoValido_StateEliminadoAtomicamenteDeDragonfly(t *testing.T) {
	uc, mr, repo, _, _, _ := newTestUseCase(t)
	ctx := t.Context()

	// Pre-poblar Dragonfly con el estado OAuth
	stateKey := "{auth}:oauth:state:state-abc-123"
	populateOAuthState(mr, "state-abc-123", "code-verifier-xyz")

	// Verificar que existe antes
	if !mr.Exists(stateKey) {
		t.Fatalf("precondición: key %s debería existir en Dragonfly antes de Execute", stateKey)
	}

	seedExistingUser(repo, "test@proactrip.com")

	cmd := validCommand()
	_, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	// Verificar que el state fue eliminado (GET+DEL atómico one-time use)
	if mr.Exists(stateKey) {
		t.Errorf("key %s todavía existe — debería haber sido eliminada atómicamente", stateKey)
	}
}

// =============================================================================
// Escenario 4: usuario_nuevo — usuario no existe → repo.Create + evento publicado.
// =============================================================================

func TestExecute_UsuarioNuevo_CreaUsuarioYPublicaEvento(t *testing.T) {
	uc, mr, repo, oauthRepo, _, eventPub := newTestUseCase(t)
	ctx := t.Context()

	// Pre-poblar Dragonfly con el estado OAuth
	populateOAuthState(mr, "state-abc-123", "code-verifier-xyz")

	// NO pre-poblar usuario → GetByEmail devuelve ErrUserNotFound

	cmd := validCommand()
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	// Verificar que el usuario fue creado
	if len(repo.createCalls) != 1 {
		t.Fatalf("repo.Create fue llamado %d veces, want 1", len(repo.createCalls))
	}
	createdUser := repo.createCalls[0]
	if createdUser.Email != "test@proactrip.com" {
		t.Errorf("createdUser.Email = %q, want %q", createdUser.Email, "test@proactrip.com")
	}
	if createdUser.Status != domain.StatusActive {
		t.Errorf("createdUser.Status = %q, want %q (email ya verificado vía OAuth)", createdUser.Status, domain.StatusActive)
	}

	// Verificar que la identidad fue creada
	if len(oauthRepo.createCalls) != 1 {
		t.Errorf("oauthRepo.CreateAuthIdentity fue llamado %d veces, want 1", len(oauthRepo.createCalls))
	}

	// Verificar que el evento fue publicado
	if len(eventPub.published) != 1 {
		t.Fatalf("eventPublisher.Publish fue llamado %d veces, want 1", len(eventPub.published))
	}
	evt := eventPub.published[0]
	wantStream := eventbus.StreamName("auth.user.registered")
	if evt.stream != wantStream {
		t.Errorf("event stream = %q, want %q", evt.stream, wantStream)
	}
	if evt.payload["event_type"] != "user_registered" {
		t.Errorf("event_type = %v, want %q", evt.payload["event_type"], "user_registered")
	}
	if evt.payload["email"] != "test@proactrip.com" {
		t.Errorf("event email = %v, want %q", evt.payload["email"], "test@proactrip.com")
	}
	if evt.payload["provider"] != "google" {
		t.Errorf("event provider = %v, want %q", evt.payload["provider"], "google")
	}

	// Verificar que los nuevos campos OAuth están presentes
	if v, ok := evt.payload["last_name"]; !ok {
		t.Error("event payload missing last_name")
	} else if v != "User" {
		t.Errorf("last_name = %v, want %q", v, "User")
	}
	if v, ok := evt.payload["locale"]; !ok {
		t.Error("event payload missing locale")
	} else if v != "es-419" {
		t.Errorf("locale = %v, want %q", v, "es-419")
	}

	// Verificar que la respuesta contiene el nuevo usuario
	if resp.User == nil {
		t.Fatal("User response es nil")
	}
	if resp.User.UserID != createdUser.ID {
		t.Errorf("UserID = %v, want %v", resp.User.UserID, createdUser.ID)
	}
}

// =============================================================================
// Escenario 5: usuario_existente — usuario conocido → NO repo.Create, RecordLogin,
// identidad actualizada, evento NO publicado.
// =============================================================================

func TestExecute_UsuarioExistente_NoCreaUsuario_RegistraLogin_ActualizaIdentidad_NoPublicaEvento(t *testing.T) {
	uc, mr, repo, oauthRepo, _, eventPub := newTestUseCase(t)
	ctx := t.Context()

	// Pre-poblar Dragonfly con el estado OAuth
	populateOAuthState(mr, "state-abc-123", "code-verifier-xyz")

	// Pre-poblar usuario existente
	existingUser := seedExistingUser(repo, "test@proactrip.com")

	// Pre-poblar identidad existente (simula que el usuario ya se autenticó antes con Google)
	existingIdentity := &domain.AuthIdentity{
		ID:             uuid.Must(uuid.NewV7()),
		UserID:         existingUser.ID,
		ProviderCode:   "google",
		ProviderUserID: "google-sub-123",
		Email:          "test@proactrip.com",
		DisplayName:    "Old Name",
		AvatarURL:      "https://old-avatar.com/pic.png",
	}
	oauthRepo.identities["google:google-sub-123"] = existingIdentity

	cmd := validCommand()
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	// Verificar que NO se creó un nuevo usuario
	if len(repo.createCalls) > 0 {
		t.Errorf("repo.Create fue llamado %d veces, se esperaba 0 (usuario ya existía)", len(repo.createCalls))
	}

	// Verificar que RecordLogin se ejecutó (repo.Update llamado)
	if len(repo.updateCalls) == 0 {
		t.Error("repo.Update no fue llamado — RecordLogin debería haberse ejecutado")
	} else {
		updatedUser := repo.updateCalls[0]
		if updatedUser.LoginCount != 1 {
			t.Errorf("LoginCount = %d, want 1", updatedUser.LoginCount)
		}
	}

	// Verificar que la identidad fue ACTUALIZADA (no creada)
	if len(oauthRepo.createCalls) > 0 {
		t.Errorf("oauthRepo.CreateAuthIdentity fue llamado, se esperaba UpdateAuthIdentity (identidad ya existía)")
	}
	if len(oauthRepo.updateCalls) != 1 {
		t.Errorf("oauthRepo.UpdateAuthIdentity fue llamado %d veces, want 1", len(oauthRepo.updateCalls))
	} else {
		updatedIdentity := oauthRepo.updateCalls[0]
		if updatedIdentity.DisplayName != "Test User" {
			t.Errorf("DisplayName = %q, want %q (debe actualizarse desde el provider)", updatedIdentity.DisplayName, "Test User")
		}
	}

	// Verificar que NO se publicó evento de registro
	if len(eventPub.published) > 0 {
		t.Errorf("eventPublisher.Publish fue llamado %d veces, se esperaba 0 (usuario ya existía)", len(eventPub.published))
	}

	// Verificar respuesta
	if resp.User == nil {
		t.Fatal("User response es nil")
	}
	if resp.User.UserID != existingUser.ID {
		t.Errorf("UserID = %v, want %v", resp.User.UserID, existingUser.ID)
	}
}

// =============================================================================
// Escenario 6: usuario_deshabilitado — usuario existe pero está disabled → error.
// AS-SPEC-007: OAuth callback rechaza cuentas deshabilitadas.
// =============================================================================

func TestExecute_UsuarioDeshabilitado_DevuelveErrAccountDisabled(t *testing.T) {
	uc, mr, repo, _, _, _ := newTestUseCase(t)
	ctx := t.Context()

	// Pre-poblar Dragonfly con el estado OAuth
	populateOAuthState(mr, "state-abc-123", "code-verifier-xyz")

	// Pre-poblar usuario existente con status disabled
	disabledUser := seedExistingUser(repo, "test@proactrip.com")
	disabledUser.Status = domain.StatusDisabled

	cmd := validCommand()
	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("Execute: se esperaba error para usuario deshabilitado, pero fue nil")
	}
	if !errors.Is(err, domain.ErrAccountDisabled) {
		t.Errorf("error = %v, want %v", err, domain.ErrAccountDisabled)
	}

	// Verificar que NO se generaron tokens (no se llegó a tokenSvc)
	// (implícito: si llegamos a tokenSvc, tendríamos respuesta con tokens)
}

// =============================================================================
// Escenario 7: usuario_suspendido — usuario existe pero está suspended → error.
// =============================================================================

func TestExecute_UsuarioSuspendido_DevuelveErrAccountSuspended(t *testing.T) {
	uc, mr, repo, _, _, _ := newTestUseCase(t)
	ctx := t.Context()

	// Pre-poblar Dragonfly con el estado OAuth
	populateOAuthState(mr, "state-abc-123", "code-verifier-xyz")

	// Pre-poblar usuario existente con status suspended
	suspendedUser := seedExistingUser(repo, "test@proactrip.com")
	suspendedUser.Status = domain.StatusSuspended

	cmd := validCommand()
	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("Execute: se esperaba error para usuario suspendido, pero fue nil")
	}
	if !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("error = %v, want %v", err, domain.ErrAccountSuspended)
	}
}

func TestExecute_StateTokenInvalido_DevuelveErrOAuthStateInvalid_SinConsultarDragonfly(t *testing.T) {
	uc, mr, _, _, _, _ := newTestUseCase(t)
	ctx := t.Context()

	// Forzar fallo del state token service
	uc.stateTokenSvc = &mockStateTokenSvc{err: errors.New("token corrupted")}

	cmd := validCommand()
	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("Execute: se esperaba error, pero fue nil")
	}
	if !errors.Is(err, domain.ErrOAuthStateInvalid) {
		t.Errorf("error = %v, want %v", err, domain.ErrOAuthStateInvalid)
	}

	// Verificar que ningún key fue creado en Dragonfly (no se llegó a GET+DEL)
	keys := mr.Keys()
	if len(keys) > 0 {
		t.Errorf("Dragonfly tiene %d keys, se esperaba 0 (el flujo falló antes de tocar Dragonfly)", len(keys))
	}
}

// =============================================================================
// Task 1.1: errorCodeFrom — extrae código de dominio de errores wrapped/unwrapped.
// =============================================================================

func TestErrorCodeFrom_DomainErrorDirecto_RetornaCodigo(t *testing.T) {
	h := &Handler{}
	code := h.errorCodeFrom(domain.ErrOAuthCodeMissing)
	if code != "OAUTH_CODE_MISSING" {
		t.Errorf("errorCodeFrom(ErrOAuthCodeMissing) = %q, want %q", code, "OAUTH_CODE_MISSING")
	}
}

func TestErrorCodeFrom_DomainErrorWrapped_RetornaCodigoDelDominio(t *testing.T) {
	h := &Handler{}
	wrapped := fmt.Errorf("crear usuario OAuth: %w", domain.ErrOAuthCodeMissing)
	code := h.errorCodeFrom(wrapped)
	if code != "OAUTH_CODE_MISSING" {
		t.Errorf("errorCodeFrom(wrapped) = %q, want %q (debe usar Unwrap, no prefix string)", code, "OAUTH_CODE_MISSING")
	}
}

func TestErrorCodeFrom_DomainErrorDobleWrap_RetornaCodigoInnermost(t *testing.T) {
	h := &Handler{}
	inner := fmt.Errorf("oauth user creation: %w", domain.ErrOAuthCodeMissing)
	outer := fmt.Errorf("callback failed: %w", inner)
	code := h.errorCodeFrom(outer)
	if code != "OAUTH_CODE_MISSING" {
		t.Errorf("errorCodeFrom(double-wrapped) = %q, want %q", code, "OAUTH_CODE_MISSING")
	}
}

func TestErrorCodeFrom_ErrorGenerico_RetornaFallback(t *testing.T) {
	h := &Handler{}
	code := h.errorCodeFrom(errors.New("unknown error"))
	if code != "OAUTH_EXCHANGE_FAILED" {
		t.Errorf("errorCodeFrom(generic) = %q, want %q", code, "OAUTH_EXCHANGE_FAILED")
	}
}

func TestErrorCodeFrom_ErrorSinCodigoDominio_WrapperSinUnwrap_RetornaFallback(t *testing.T) {
	h := &Handler{}
	// Error with colon but not a domain error (no domain sentinel in chain)
	wrapped := fmt.Errorf("some prefix: %w", errors.New("plain error"))
	code := h.errorCodeFrom(wrapped)
	if code != "OAUTH_EXCHANGE_FAILED" {
		t.Errorf("errorCodeFrom(wrapped plain error) = %q, want %q", code, "OAUTH_EXCHANGE_FAILED")
	}
}
