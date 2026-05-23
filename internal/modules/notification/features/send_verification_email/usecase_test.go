// Tests para el caso de uso de envío de email de verificación.
// Cubre: Command Validate (D2), Execute con mocks de repo+sender (D3),
// idempotencia, manejo de errores, first_name opcional.
//
// Convenciones:
//   - White-box testing (package send_verification_email).
//   - Table-driven con t.Run(), nombres de sub-tests en español.
//   - Solo stdlib testing, sin testify.
package send_verification_email

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/config"
	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
)

// =============================================================================
// Mocks
// =============================================================================

// mockRepo implementa domain.NotificationRepository para tests.
type mockRepo struct {
	saveFn     func(ctx context.Context, n *domain.Notification) (uuid.UUID, error)
	getByIDFn  func(ctx context.Context, id uuid.UUID) (*domain.Notification, error)
	markSentFn func(ctx context.Context, id uuid.UUID) error
}

func (m *mockRepo) Save(ctx context.Context, n *domain.Notification) (uuid.UUID, error) {
	if m.saveFn != nil {
		return m.saveFn(ctx, n)
	}
	return uuid.Nil, nil
}

func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockRepo) MarkSent(ctx context.Context, id uuid.UUID) error {
	if m.markSentFn != nil {
		return m.markSentFn(ctx, id)
	}
	return nil
}

// mockSender implementa EmailSender para tests.
type mockSender struct {
	sendFn func(ctx context.Context, to, templateID string, vars map[string]any) (string, error)
}

func (m *mockSender) SendWithTemplate(ctx context.Context, to, templateID string, vars map[string]any) (string, error) {
	if m.sendFn != nil {
		return m.sendFn(ctx, to, templateID, vars)
	}
	return "mock-msg-id", nil
}

// =============================================================================
// H5.1 — TestCommand_Validate: table-driven para todos los casos de validación
// =============================================================================

func TestCommand_Validate(t *testing.T) {
	tests := []struct {
		nombre  string
		cmd     Command
		wantErr bool
	}{
		{
			nombre: "comando válido con todos los campos",
			cmd: Command{
				UserID:            uuid.New(),
				Email:             "test@example.com",
				VerificationToken: "token-abc-123",
				FirstName:         "Aurelio",
			},
			wantErr: false,
		},
		{
			nombre: "comando válido sin first_name (opcional)",
			cmd: Command{
				UserID:            uuid.New(),
				Email:             "test@example.com",
				VerificationToken: "token-abc-123",
				FirstName:         "",
			},
			wantErr: false,
		},
		{
			nombre: "email vacío",
			cmd: Command{
				UserID:            uuid.New(),
				Email:             "",
				VerificationToken: "token-abc-123",
			},
			wantErr: true,
		},
		{
			nombre: "token vacío",
			cmd: Command{
				UserID:            uuid.New(),
				Email:             "test@example.com",
				VerificationToken: "",
			},
			wantErr: true,
		},
		{
			nombre: "UserID nil",
			cmd: Command{
				UserID:            uuid.Nil,
				Email:             "test@example.com",
				VerificationToken: "token-abc-123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			err := tt.cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// H5.2 — TestExecute_FlujoFeliz: envía email y llama a MarkSent
// =============================================================================

func TestExecute_FlujoFeliz(t *testing.T) {
	markSentCalled := false
	repo := &mockRepo{
		markSentFn: func(ctx context.Context, id uuid.UUID) error {
			markSentCalled = true
			return nil
		},
	}
	sender := &mockSender{
		sendFn: func(ctx context.Context, to, templateID string, vars map[string]any) (string, error) {
			return "resend_flow_ok", nil
		},
	}

	uc := NewUseCase(Deps{
		Repo:           repo,
		Sender:         sender,
		FrontendConfig: config.FrontendConfig{},
	})

	cmd := Command{
		UserID:            uuid.New(),
		Email:             "user@example.com",
		VerificationToken: "valid-token",
		FirstName:         "Aurelio",
	}

	ctx := t.Context()
	err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute inesperadamente falló: %v", err)
	}
	if !markSentCalled {
		t.Error("MarkSent debería haber sido llamado después del envío exitoso")
	}
}

// =============================================================================
// H5.3 — TestExecute_ComandoInvalido: retorna error con comando vacío
// =============================================================================

func TestExecute_ComandoInvalido(t *testing.T) {
	uc := NewUseCase(Deps{
		Repo:   &mockRepo{},
		Sender: &mockSender{},
	})

	cmd := Command{} // UserID nil, email vacío, token vacío
	err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error por comando inválido")
	}
}

// =============================================================================
// H5.4 — TestExecute_ErrorDeRepoSave: propaga error del repositorio
// =============================================================================

func TestExecute_ErrorDeRepoSave(t *testing.T) {
	repo := &mockRepo{
		saveFn: func(ctx context.Context, n *domain.Notification) (uuid.UUID, error) {
			return uuid.Nil, errors.New("DB connection lost")
		},
	}

	uc := NewUseCase(Deps{
		Repo:           repo,
		Sender:         &mockSender{},
		FrontendConfig: config.FrontendConfig{},
	})

	cmd := Command{
		UserID:            uuid.New(),
		Email:             "user@example.com",
		VerificationToken: "token",
	}

	err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error del repositorio, se obtuvo nil")
	}
}

// =============================================================================
// H5.5 — TestExecute_ErrorDeSender: falla el envío y loguea error
// =============================================================================

func TestExecute_ErrorDeSender(t *testing.T) {
	repo := &mockRepo{}
	sender := &mockSender{
		sendFn: func(ctx context.Context, to, templateID string, vars map[string]any) (string, error) {
			return "", errors.New("Resend API timeout")
		},
	}

	uc := NewUseCase(Deps{
		Repo:           repo,
		Sender:         sender,
		FrontendConfig: config.FrontendConfig{},
	})

	cmd := Command{
		UserID:            uuid.New(),
		Email:             "user@example.com",
		VerificationToken: "token",
	}

	err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error del sender, se obtuvo nil")
	}
}

// =============================================================================
// H5.6 — TestExecute_Idempotencia: notificación ya enviada retorna nil
// =============================================================================

func TestExecute_Idempotencia(t *testing.T) {
	existingID := uuid.New()
	sendCalled := false
	now := time.Now()
	sentAt := &now

	repo := &mockRepo{
		saveFn: func(ctx context.Context, n *domain.Notification) (uuid.UUID, error) {
			return existingID, nil // Ya existe
		},
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Notification, error) {
			return &domain.Notification{
				ID:     existingID,
				SentAt: sentAt,
			}, nil
		},
	}
	sender := &mockSender{
		sendFn: func(ctx context.Context, to, templateID string, vars map[string]any) (string, error) {
			sendCalled = true
			return "should-not-reach", nil
		},
	}

	uc := NewUseCase(Deps{
		Repo:           repo,
		Sender:         sender,
		FrontendConfig: config.FrontendConfig{},
	})

	cmd := Command{
		UserID:            uuid.New(),
		Email:             "user@example.com",
		VerificationToken: "token",
	}

	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("Execute falló en caso de idempotencia: %v", err)
	}
	if sendCalled {
		t.Error("SendWithTemplate fue llamado en un caso de idempotencia (no debería)")
	}
}

// =============================================================================
// H5.7 — TestExecute_FirstNameOpcional: envía sin first_name cuando está vacío
// =============================================================================

func TestExecute_FirstNameOpcional(t *testing.T) {
	var capturedVars map[string]any
	sender := &mockSender{
		sendFn: func(ctx context.Context, to, templateID string, vars map[string]any) (string, error) {
			capturedVars = vars
			return "msg-firstname-empty", nil
		},
	}

	uc := NewUseCase(Deps{
		Repo:           &mockRepo{},
		Sender:         sender,
		FrontendConfig: config.FrontendConfig{},
	})

	cmd := Command{
		UserID:            uuid.New(),
		Email:             "user@example.com",
		VerificationToken: "token",
		FirstName:         "", // Vacío
	}

	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, hasFirstName := capturedVars["first_name"]; hasFirstName {
		t.Error("first_name no debería estar en las variables del template cuando está vacío")
	}
	if _, hasURL := capturedVars["verification_url"]; !hasURL {
		t.Error("verification_url debería estar presente siempre")
	}
}
