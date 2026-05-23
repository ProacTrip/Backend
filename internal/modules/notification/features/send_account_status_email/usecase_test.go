// Tests for send_account_status_email usecase.
package send_account_status_email_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
	sendstatus "github.com/ProacTrip/Backend/internal/modules/notification/features/send_account_status_email"
)

// =============================================================================
// Stubs para dependencias del usecase
// =============================================================================

// stubEmailSender implementa send_account_status_email.EmailSender para tests.
type stubEmailSender struct {
	shouldErr    error
	lastTo       string
	lastTemplate string
	lastVars     map[string]any
	callCount    int
}

func (s *stubEmailSender) SendWithTemplate(_ context.Context, to, templateID string, vars map[string]any) (string, error) {
	if s.shouldErr != nil {
		return "", s.shouldErr
	}
	s.callCount++
	s.lastTo = to
	s.lastTemplate = templateID
	s.lastVars = vars
	return "msg_resend_stub_001", nil
}

// stubNotificationRepo implementa domain.NotificationRepository para tests.
type stubNotificationRepo struct {
	saved         *domain.Notification
	existingID    uuid.UUID
	saveShouldErr error
	getByIDResult *domain.Notification
	getByIDErr    error
	markSentErr   error
	markSentCalls int
}

func (r *stubNotificationRepo) Save(_ context.Context, n *domain.Notification) (uuid.UUID, error) {
	if r.saveShouldErr != nil {
		return uuid.Nil, r.saveShouldErr
	}
	r.saved = n
	return r.existingID, nil
}

func (r *stubNotificationRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Notification, error) {
	return r.getByIDResult, r.getByIDErr
}

func (r *stubNotificationRepo) MarkSent(_ context.Context, _ uuid.UUID) error {
	r.markSentCalls++
	return r.markSentErr
}

// Compile-time check
var _ domain.NotificationRepository = (*stubNotificationRepo)(nil)

// =============================================================================
// Helpers
// =============================================================================

func newUseCase(repo domain.NotificationRepository, sender sendstatus.EmailSender) *sendstatus.UseCase {
	return sendstatus.NewUseCase(sendstatus.Deps{
		Repo:   repo,
		Sender: sender,
	})
}

// =============================================================================
// Tests de validación
// =============================================================================

// TestValidate_SinEmail rechaza comando sin email.
func TestValidate_SinEmail(t *testing.T) {
	cmd := sendstatus.Command{
		UserID:     uuid.Must(uuid.NewV7()),
		Email:      "",
		TemplateID: sendstatus.TemplateAccountDisabled,
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for missing Email, got nil")
	}
}

// TestValidate_SinUserID rechaza comando con UserID nil.
func TestValidate_SinUserID(t *testing.T) {
	cmd := sendstatus.Command{
		UserID:     uuid.Nil,
		Email:      "test@example.com",
		TemplateID: sendstatus.TemplateAccountDisabled,
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for nil UserID, got nil")
	}
}

// TestValidate_SinTemplateID rechaza comando sin TemplateID.
func TestValidate_SinTemplateID(t *testing.T) {
	cmd := sendstatus.Command{
		UserID:     uuid.Must(uuid.NewV7()),
		Email:      "test@example.com",
		TemplateID: "",
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for missing TemplateID, got nil")
	}
}

// =============================================================================
// Tests de happy path
// =============================================================================

// TestExecute_Disabled_HappyPath verifica el envío exitoso del email de cuenta deshabilitada.
func TestExecute_Disabled_HappyPath(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	sender := &stubEmailSender{}
	repo := &stubNotificationRepo{}
	uc := newUseCase(repo, sender)

	cmd := sendstatus.Command{
		UserID:     userID,
		Email:      "disabled@example.com",
		TemplateID: sendstatus.TemplateAccountDisabled,
	}

	err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Verificar que el sender fue llamado con los parámetros correctos
	if sender.callCount != 1 {
		t.Errorf("sender called %d times, want 1", sender.callCount)
	}
	if sender.lastTo != "disabled@example.com" {
		t.Errorf("sender.to = %q, want %q", sender.lastTo, "disabled@example.com")
	}
	if sender.lastTemplate != sendstatus.TemplateAccountDisabled {
		t.Errorf("sender.template = %q, want %q", sender.lastTemplate, sendstatus.TemplateAccountDisabled)
	}
	if sender.lastVars["user_email"] != "disabled@example.com" {
		t.Errorf("user_email in vars = %v, want 'disabled@example.com'", sender.lastVars["user_email"])
	}

	// Verificar que la notificación fue guardada con los campos correctos
	if repo.saved == nil {
		t.Fatal("notification was not saved")
	}
	if repo.saved.TemplateCode != "account_disabled" {
		t.Errorf("template_code = %q, want %q", repo.saved.TemplateCode, "account_disabled")
	}

	// Verificar que MarkSent fue llamado
	if repo.markSentCalls != 1 {
		t.Errorf("MarkSent called %d times, want 1", repo.markSentCalls)
	}
}

// TestExecute_Enabled_HappyPath verifica el envío exitoso del email de cuenta habilitada.
func TestExecute_Enabled_HappyPath(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	sender := &stubEmailSender{}
	repo := &stubNotificationRepo{}
	uc := newUseCase(repo, sender)

	cmd := sendstatus.Command{
		UserID:     userID,
		Email:      "enabled@example.com",
		TemplateID: sendstatus.TemplateAccountEnabled,
	}

	err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if sender.lastTemplate != sendstatus.TemplateAccountEnabled {
		t.Errorf("sender.template = %q, want %q", sender.lastTemplate, sendstatus.TemplateAccountEnabled)
	}
	if sender.lastVars["user_email"] != "enabled@example.com" {
		t.Errorf("user_email in vars = %v, want 'enabled@example.com'", sender.lastVars["user_email"])
	}
	if repo.saved.TemplateCode != "account_enabled" {
		t.Errorf("template_code = %q, want %q", repo.saved.TemplateCode, "account_enabled")
	}
}

// =============================================================================
// Tests de error del sender
// =============================================================================

// TestExecute_SenderError verifica que cuando el sender falla, el error se propaga.
func TestExecute_SenderError(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	sender := &stubEmailSender{
		shouldErr: fmt.Errorf("Resend API error: timeout"),
	}
	repo := &stubNotificationRepo{}
	uc := newUseCase(repo, sender)

	cmd := sendstatus.Command{
		UserID:     userID,
		Email:      "fail@example.com",
		TemplateID: sendstatus.TemplateAccountDisabled,
	}

	err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error from sender, got nil")
	}
}

// =============================================================================
// Tests de idempotencia
// =============================================================================

// TestExecute_Idempotency_AlreadySent verifica que si la notificación ya fue
// enviada, se retorna nil sin reenviar.
func TestExecute_Idempotency_AlreadySent(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	existingID := uuid.Must(uuid.NewV7())
	sentAt := time.Now()
	sender := &stubEmailSender{}
	repo := &stubNotificationRepo{
		existingID: existingID,
		getByIDResult: &domain.Notification{
			ID:     existingID,
			SentAt: &sentAt,
		},
	}
	uc := newUseCase(repo, sender)

	cmd := sendstatus.Command{
		UserID:     userID,
		Email:      "idempotent@example.com",
		TemplateID: sendstatus.TemplateAccountDisabled,
	}

	err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// El sender NO debe ser llamado porque la notificación ya fue enviada
	if sender.callCount > 0 {
		t.Errorf("sender should NOT be called for already-sent notification, but was called %d times", sender.callCount)
	}
}

// =============================================================================
// Test de interacción
// =============================================================================

// TestExecute_SenderCalledWithCorrectTemplate verifica que el sender es invocado
// con el template ID correcto. El rate limiting se maneja en el adapter (ResendService),
// no en el usecase. Este test confirma que el usecase pasa el template ID correcto.
func TestExecute_SenderCalledWithCorrectTemplate(t *testing.T) {
	ctx := t.Context()

	userID := uuid.Must(uuid.NewV7())
	sender := &stubEmailSender{}
	repo := &stubNotificationRepo{}
	uc := newUseCase(repo, sender)

	cmd := sendstatus.Command{
		UserID:     userID,
		Email:      "ratelimit-test@example.com",
		TemplateID: sendstatus.TemplateAccountDisabled,
	}

	err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if sender.callCount != 1 {
		t.Errorf("sender called %d times, want 1", sender.callCount)
	}
	if sender.lastTemplate != sendstatus.TemplateAccountDisabled {
		t.Errorf("sender.template = %q, want %q", sender.lastTemplate, sendstatus.TemplateAccountDisabled)
	}
	if sender.lastVars["user_email"] != "ratelimit-test@example.com" {
		t.Errorf("user_email in vars = %v, want 'ratelimit-test@example.com'", sender.lastVars["user_email"])
	}
}
