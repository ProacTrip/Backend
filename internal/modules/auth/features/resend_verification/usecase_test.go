package resend_verification

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Mocks para tests del usecase de resend_verification (T1.7)
// =============================================================================

type mockUserRepo struct {
	user       *domain.User
	getByErr   error
	getByCalled bool
}

func (m *mockUserRepo) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	m.getByCalled = true
	if m.getByErr != nil {
		return nil, m.getByErr
	}
	return m.user, nil
}

// Stubs para métodos no utilizados del UserRepository
func (m *mockUserRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.User, error) {
	return nil, nil
}
func (m *mockUserRepo) Update(_ context.Context, _ *domain.User) error { return nil }
func (m *mockUserRepo) Create(_ context.Context, _ *domain.User) error { return nil }
func (m *mockUserRepo) GetRoleByName(_ context.Context, _ string) (*domain.Role, error) {
	return nil, nil
}

type mockVerificationTokenService struct {
	token         string
	err           error
	generateCalled bool
}

func (m *mockVerificationTokenService) GenerateToken(_ context.Context, _ string) (string, error) {
	m.generateCalled = true
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}

type mockNotificationPort struct {
	err        error
	sendCalled bool
	lastEmail  string
	lastToken  string
}

func (m *mockNotificationPort) SendVerificationEmail(_ context.Context, _ uuid.UUID, email, verificationToken string) error {
	m.sendCalled = true
	m.lastEmail = email
	m.lastToken = verificationToken
	return m.err
}

// =============================================================================
// Helpers
// =============================================================================

func newResendUseCase(repo *mockUserRepo, tokenSvc *mockVerificationTokenService, notifier *mockNotificationPort) *UseCase {
	return NewUseCase(UseCaseDeps{
		Repo:     repo,
		TokenSvc: tokenSvc,
		Notifier: notifier,
	})
}

func newTestUser(verified bool) *domain.User {
	return &domain.User{
		ID:            uuid.Must(uuid.NewV7()),
		Email:         "test@example.com",
		EmailVerified: verified,
		RoleID:        uuid.Must(uuid.NewV7()),
		RoleName:      "client",
		Status:        domain.StatusActive,
	}
}

// =============================================================================
// Tests — Anti-enumeration: todos los escenarios retornan nil error + mensaje
// genérico. Nunca se revela si el email existe o no.
// =============================================================================

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 1: email_no_existe — GetByEmail retorna ErrUserNotFound.
// Respuesta: 200 OK con mensaje genérico. Sin error.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_EmailNoExiste_Retorna200Generico(t *testing.T) {
	repo := &mockUserRepo{getByErr: domain.ErrUserNotFound}
	tokenSvc := &mockVerificationTokenService{}
	notifier := &mockNotificationPort{}

	uc := newResendUseCase(repo, tokenSvc, notifier)

	resp, err := uc.Execute(t.Context(), Command{Email: "noexiste@example.com"})
	if err != nil {
		t.Fatalf("Execute() NO debería retornar error (anti-enumeración), obtuvo: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}
	if resp.Message != DefaultResponse {
		t.Errorf("mensaje = %q, se esperaba %q", resp.Message, DefaultResponse)
	}

	// No debe intentar generar token ni enviar email
	if tokenSvc.generateCalled {
		t.Error("GenerateToken() NO debería haberse llamado cuando el email no existe")
	}
	if notifier.sendCalled {
		t.Error("SendVerificationEmail() NO debería haberse llamado cuando el email no existe")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 2: email_ya_verificado — el usuario existe y EmailVerified=true.
// Respuesta: 200 OK genérico. No se genera token ni se envía email.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_EmailYaVerificado_Retorna200SinEnvio(t *testing.T) {
	user := newTestUser(true)
	repo := &mockUserRepo{user: user}
	tokenSvc := &mockVerificationTokenService{}
	notifier := &mockNotificationPort{}

	uc := newResendUseCase(repo, tokenSvc, notifier)

	resp, err := uc.Execute(t.Context(), Command{Email: "test@example.com"})
	if err != nil {
		t.Fatalf("Execute() NO debería retornar error, obtuvo: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}
	if resp.Message != DefaultResponse {
		t.Errorf("mensaje = %q, se esperaba %q", resp.Message, DefaultResponse)
	}

	// No debe generar token ni enviar email porque ya está verificado
	if tokenSvc.generateCalled {
		t.Error("GenerateToken() NO debería haberse llamado cuando el email ya está verificado")
	}
	if notifier.sendCalled {
		t.Error("SendVerificationEmail() NO debería haberse llamado cuando el email ya está verificado")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 3: email_no_verificado — usuario existe, EmailVerified=false.
// Se genera token nuevo y se envía email de verificación.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_EmailNoVerificado_GeneraTokenYEnviaEmail(t *testing.T) {
	user := newTestUser(false)
	repo := &mockUserRepo{user: user}
	tokenSvc := &mockVerificationTokenService{token: "new-verification-token"}
	notifier := &mockNotificationPort{}

	uc := newResendUseCase(repo, tokenSvc, notifier)

	resp, err := uc.Execute(t.Context(), Command{Email: "test@example.com"})
	if err != nil {
		t.Fatalf("Execute() error inesperado: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}
	if resp.Message != DefaultResponse {
		t.Errorf("mensaje = %q, se esperaba %q", resp.Message, DefaultResponse)
	}

	// Debe haber generado token
	if !tokenSvc.generateCalled {
		t.Error("GenerateToken() debería haberse llamado para email no verificado")
	}

	// Debe haber enviado email
	if !notifier.sendCalled {
		t.Error("SendVerificationEmail() debería haberse llamado para email no verificado")
	}
	if notifier.lastEmail != "test@example.com" {
		t.Errorf("email enviado a = %q, se esperaba %q", notifier.lastEmail, "test@example.com")
	}
	if notifier.lastToken != "new-verification-token" {
		t.Errorf("token enviado = %q, se esperaba %q", notifier.lastToken, "new-verification-token")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 4: error_infraestructura — GetByEmail retorna error inesperado
// (no ErrUserNotFound). El usecase loguea y retorna 200 genérico.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_ErrorInfraestructura_Retorna200Generico(t *testing.T) {
	repo := &mockUserRepo{getByErr: errors.New("DB connection refused")}
	tokenSvc := &mockVerificationTokenService{}
	notifier := &mockNotificationPort{}

	uc := newResendUseCase(repo, tokenSvc, notifier)

	resp, err := uc.Execute(t.Context(), Command{Email: "test@example.com"})
	if err != nil {
		t.Fatalf("Execute() NO debería retornar error ante falla de infra, obtuvo: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}
	if resp.Message != DefaultResponse {
		t.Errorf("mensaje = %q, se esperaba %q", resp.Message, DefaultResponse)
	}

	// No debe intentar generar token ni enviar email tras error de infra
	if tokenSvc.generateCalled {
		t.Error("GenerateToken() NO debería haberse llamado tras error de infraestructura")
	}
	if notifier.sendCalled {
		t.Error("SendVerificationEmail() NO debería haberse llamado tras error de infraestructura")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Escenario 5: error_notificacion — el token se genera correctamente pero
// SendVerificationEmail falla. El usecase retorna 200 genérico sin exponer
// el error de notificación.
// ─────────────────────────────────────────────────────────────────────────────

func TestExecute_ErrorNotificacion_Retorna200Generico(t *testing.T) {
	user := newTestUser(false)
	repo := &mockUserRepo{user: user}
	tokenSvc := &mockVerificationTokenService{token: "verification-token-xyz"}
	notifier := &mockNotificationPort{err: errors.New("SMTP connection timeout")}

	uc := newResendUseCase(repo, tokenSvc, notifier)

	resp, err := uc.Execute(t.Context(), Command{Email: "test@example.com"})
	if err != nil {
		t.Fatalf("Execute() NO debería retornar error ante falla de notificación, obtuvo: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() devolvió respuesta nil")
	}
	if resp.Message != DefaultResponse {
		t.Errorf("mensaje = %q, se esperaba %q", resp.Message, DefaultResponse)
	}

	// El token sí se generó (el error es posterior, en la notificación)
	if !tokenSvc.generateCalled {
		t.Error("GenerateToken() debería haberse llamado (el error es en notificación, no en generación)")
	}

	// Se intentó enviar el email pero falló
	if !notifier.sendCalled {
		t.Error("SendVerificationEmail() debería haberse llamado (aunque falle)")
	}
}
