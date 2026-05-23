package register

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/verification"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Mock types for UseCase tests
// =============================================================================

type mockUserRepo struct {
	users     map[string]*domain.User
	roles     map[string]*domain.Role
	getByErr  error
	createErr error
	roleErr   error
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

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if m.getByErr != nil {
		return nil, m.getByErr
	}
	u, ok := m.users[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) Create(_ context.Context, user *domain.User) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.users[user.Email] = user
	return nil
}

func (m *mockUserRepo) GetRoleByName(_ context.Context, name string) (*domain.Role, error) {
	if m.roleErr != nil {
		return nil, m.roleErr
	}
	r, ok := m.roles[name]
	if !ok {
		return nil, domain.ErrRoleNotFound
	}
	return r, nil
}

func (m *mockUserRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.User, error) { return nil, nil }
func (m *mockUserRepo) Update(_ context.Context, _ *domain.User) error               { return nil }

type mockPasswordHasher struct{}

func (m *mockPasswordHasher) Hash(password string) (string, error)  { return "hashed:" + password, nil }
func (m *mockPasswordHasher) Verify(_, _ string) (bool, error)      { return true, nil }

type mockVerificationService struct {
	token string
	err   error
}

func (m *mockVerificationService) GenerateToken(_ context.Context, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}
func (m *mockVerificationService) VerifyToken(_ context.Context, _ string) (*verification.TokenClaims, error) {
	return nil, nil
}

type mockEventPublisher struct {
	published []publishedEvent
}

type publishedEvent struct {
	stream  string
	payload map[string]any
}

func (m *mockEventPublisher) Publish(_ context.Context, stream string, payload map[string]any) (string, error) {
	m.published = append(m.published, publishedEvent{stream: stream, payload: payload})
	return "msg-1", nil
}

// =============================================================================
// Test: flujo feliz — crea usuario, publica evento con campos de entorno vacíos.
// El usecase ya no resuelve environment defaults — publica evento mínimo.
// =============================================================================

func TestUseCase_Execute_Success(t *testing.T) {
	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "verify-token-123"},
		Hasher:         &mockPasswordHasher{},
		EventPublisher: publisher,
	})

	resp, err := uc.Execute(t.Context(), Command{
		Email:     "test@example.com",
		Password:  "Password123!",
		FirstName: "Juan",
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() returned nil response")
	}

	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(publisher.published))
	}

	payload := publisher.published[0].payload
	if payload["email"] != "test@example.com" {
		t.Errorf("email = %q, want %q", payload["email"], "test@example.com")
	}
	if payload["first_name"] != "Juan" {
		t.Errorf("first_name = %q, want %q", payload["first_name"], "Juan")
	}

	// Los campos de entorno no se incluyen — el user consumer los resuelve por su cuenta.
	if _, ok := payload["language_code"]; ok {
		t.Errorf("language_code should NOT be in event payload (resolved by user consumer)")
	}
	if _, ok := payload["currency_code"]; ok {
		t.Errorf("currency_code should NOT be in event payload")
	}

	// Response contiene solo el mensaje (sin tokens ni cookies).
	if resp.Message != "Registration successful. Please verify your email." {
		t.Errorf("Message = %q", resp.Message)
	}
}

// =============================================================================
// Test: email ya existe → ErrEmailAlreadyExists
// =============================================================================

func TestUseCase_EmailAlreadyExists(t *testing.T) {
	repo := newMockUserRepo()
	// Pre-populate the user
	existingUser := domain.NewUser("exists@example.com", "hashed", "Existing", uuid.Must(uuid.NewV7()))
	repo.users["exists@example.com"] = existingUser

	uc := NewUseCase(UseCaseDeps{
		Repo:      repo,
		VerifySvc: &mockVerificationService{token: "vt"},
		Hasher:    &mockPasswordHasher{},
	})

	_, err := uc.Execute(t.Context(), Command{
		Email:    "exists@example.com",
		Password: "Password123!",
	})
	if err == nil {
		t.Fatal("Execute() should return error for duplicate email")
	}
	if err != domain.ErrEmailAlreadyExists {
		t.Errorf("error = %v, want ErrEmailAlreadyExists", err)
	}
}

// =============================================================================
// Test: repo error → propagado
// =============================================================================

func TestUseCase_RepoError(t *testing.T) {
	repo := newMockUserRepo()
	repo.getByErr = errors.New("db connection failed")

	uc := NewUseCase(UseCaseDeps{
		Repo:      repo,
		VerifySvc: &mockVerificationService{token: "vt"},
		Hasher:    &mockPasswordHasher{},
	})

	_, err := uc.Execute(t.Context(), Command{
		Email:    "error@example.com",
		Password: "Password123!",
	})
	if err == nil {
		t.Fatal("Execute() should return error when repo fails")
	}
}

// =============================================================================
// Test: nil event publisher → no panic, registro exitoso
// =============================================================================

func TestUseCase_NilPublisher_NoPanic(t *testing.T) {
	repo := newMockUserRepo()

	uc := NewUseCase(UseCaseDeps{
		Repo:      repo,
		VerifySvc: &mockVerificationService{token: "vt-nil-pub"},
		Hasher:    &mockPasswordHasher{},
		// EventPublisher: nil — explícitamente nil
	})

	resp, err := uc.Execute(t.Context(), Command{
		Email:    "nilpub@example.com",
		Password: "Password123!",
	})
	if err != nil {
		t.Fatalf("Execute() should NOT fail with nil publisher: %v", err)
	}
	if resp == nil {
		t.Fatal("Execute() returned nil response")
	}
}
