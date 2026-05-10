package verify_email

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Mocks para tests del usecase de verify_email (T1.6)
// =============================================================================

type mockVerificationService struct {
	claims *verification.TokenClaims
	err    error
}

func (m *mockVerificationService) VerifyToken(_ context.Context, _ string) (*verification.TokenClaims, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.claims, nil
}

type mockUserRepo struct {
	user         *domain.User
	getByErr     error
	updateErr    error
	updateCalled bool
}

func (m *mockUserRepo) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	if m.getByErr != nil {
		return nil, m.getByErr
	}
	return m.user, nil
}

func (m *mockUserRepo) Update(_ context.Context, _ *domain.User) error {
	m.updateCalled = true
	return m.updateErr
}

// Stubs para métodos no utilizados del UserRepository
func (m *mockUserRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.User, error) {
	return nil, nil
}
func (m *mockUserRepo) Create(_ context.Context, _ *domain.User) error { return nil }
func (m *mockUserRepo) GetRoleByName(_ context.Context, _ string) (*domain.Role, error) {
	return nil, nil
}

type mockTokenService struct {
	pair *token.TokenPair
	err  error
}

func (m *mockTokenService) GenerateTokenPair(_ uuid.UUID, _ string, _ string, _ uuid.UUID, _ uuid.UUID) (*token.TokenPair, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pair, nil
}

// =============================================================================
// Helpers
// =============================================================================

func newVerifyEmailUseCase(verifySvc *mockVerificationService, repo *mockUserRepo, tokenSvc *mockTokenService) *UseCase {
	return NewUseCase(UseCaseDeps{
		VerifySvc: verifySvc,
		Repo:      repo,
		TokenSvc:  tokenSvc,
	})
}

func newTestUser(verified bool) *domain.User {
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	return &domain.User{
		ID:            userID,
		Email:         "test@example.com",
		EmailVerified: verified,
		RoleID:        roleID,
		RoleName:      "client",
		Status:        domain.StatusActive,
	}
}

func defaultTokenPair() *token.TokenPair {
	return &token.TokenPair{
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
	}
}

// =============================================================================
// Tests
// =============================================================================

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 1: token_valido — flujo feliz: verifica token, busca usuario,
// marca como verificado, actualiza repo y genera tokens.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_TokenValido_FlujoCompletoExitoso(t *testing.T) {
	verifySvc := &mockVerificationService{
		claims: &verification.TokenClaims{
			Email: "test@example.com",
			JTI:   uuid.Must(uuid.NewV7()),
		},
	}

	user := newTestUser(false)
	repo := &mockUserRepo{user: user}

	tokenSvc := &mockTokenService{pair: defaultTokenPair()}

	uc := newVerifyEmailUseCase(verifySvc, repo, tokenSvc)

	resp, err := uc.Execute(t.Context(), Command{Token: "valid-verification-token"})
	if err != nil {
		t.Fatalf("Execute() error inesperado: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}

	// VerifyEmail fue llamado → EmailVerified debe ser true
	if !user.EmailVerified {
		t.Error("EmailVerified = false, se esperaba true después de VerifyEmail()")
	}

	// Update fue llamado
	if !repo.updateCalled {
		t.Error("Update() debería haber sido llamado")
	}

	// Tokens generados
	if resp.AccessToken != "access-token-abc" {
		t.Errorf("AccessToken = %q, se esperaba %q", resp.AccessToken, "access-token-abc")
	}
	if resp.RefreshToken != "refresh-token-xyz" {
		t.Errorf("RefreshToken = %q, se esperaba %q", resp.RefreshToken, "refresh-token-xyz")
	}

	// Respuesta del usuario
	if resp.User.Email != "test@example.com" {
		t.Errorf("User.Email = %q, se esperaba %q", resp.User.Email, "test@example.com")
	}
	if !resp.User.EmailVerified {
		t.Error("User.EmailVerified = false en respuesta, se esperaba true")
	}
	if resp.User.RoleName != "client" {
		t.Errorf("User.RoleName = %q, se esperaba %q", resp.User.RoleName, "client")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 2: token_expirado — el servicio de verificación retorna error.
// El usecase envuelve como ErrTokenInvalid.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_TokenExpirado_RetornaErrTokenInvalid(t *testing.T) {
	verifySvc := &mockVerificationService{
		err: errors.New("token expirado"),
	}

	repo := &mockUserRepo{}
	tokenSvc := &mockTokenService{}

	uc := newVerifyEmailUseCase(verifySvc, repo, tokenSvc)

	_, err := uc.Execute(t.Context(), Command{Token: "token-expirado"})
	if err == nil {
		t.Fatal("Execute() debería retornar error para token expirado")
	}
	if !errors.Is(err, domain.ErrTokenInvalid) {
		t.Errorf("error = %v, se esperaba ErrTokenInvalid", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 3: usuario_no_encontrado — GetByEmail retorna ErrUserNotFound.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_UsuarioNoEncontrado_RetornaErrUserNotFound(t *testing.T) {
	verifySvc := &mockVerificationService{
		claims: &verification.TokenClaims{
			Email: "noexiste@example.com",
			JTI:   uuid.Must(uuid.NewV7()),
		},
	}

	repo := &mockUserRepo{getByErr: domain.ErrUserNotFound}
	tokenSvc := &mockTokenService{}

	uc := newVerifyEmailUseCase(verifySvc, repo, tokenSvc)

	_, err := uc.Execute(t.Context(), Command{Token: "valid-token"})
	if err == nil {
		t.Fatal("Execute() debería retornar error para usuario no encontrado")
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("error = %v, se esperaba ErrUserNotFound", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 4: ya_verificado — el email ya está verificado. El usecase actual
// no tiene early-return para este caso (TODO: optimizar), así que
// VerifyEmail() y Update() se ejecutan de forma idempotente y los tokens
// se generan correctamente.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_YaVerificado_FlujoIdempotenteExitoso(t *testing.T) {
	verifySvc := &mockVerificationService{
		claims: &verification.TokenClaims{
			Email: "test@example.com",
			JTI:   uuid.Must(uuid.NewV7()),
		},
	}

	user := newTestUser(true) // ya verificado
	repo := &mockUserRepo{user: user}
	tokenSvc := &mockTokenService{pair: defaultTokenPair()}

	uc := newVerifyEmailUseCase(verifySvc, repo, tokenSvc)

	resp, err := uc.Execute(t.Context(), Command{Token: "valid-token"})
	if err != nil {
		t.Fatalf("Execute() error inesperado para usuario ya verificado: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}

	// VerifyEmail es idempotente: EmailVerified sigue siendo true
	if !user.EmailVerified {
		t.Error("EmailVerified = false, debería seguir siendo true")
	}
	if user.Status != domain.StatusActive {
		t.Errorf("Status = %q, se esperaba %q", user.Status, domain.StatusActive)
	}

	// Update fue llamado (comportamiento actual; TODO: early-return para
	// evitar escritura innecesaria cuando EmailVerified ya es true)
	if !repo.updateCalled {
		t.Error("Update() debería haber sido llamado (comportamiento actual)")
	}

	// Tokens generados
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("tokens deberían haberse generado incluso para usuario ya verificado")
	}
	if resp.User.EmailVerified != true {
		t.Error("User.EmailVerified en respuesta debería ser true")
	}
}
