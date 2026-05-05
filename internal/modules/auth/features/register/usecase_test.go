package register

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
// Mock types for UseCase tests (Task 2.2)
// =============================================================================

type mockUserRepo struct {
	users     map[string]*domain.User // email → user
	roles     map[string]*domain.Role // name → role
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
	m.users[user.Email] = user
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

// Remaining interface methods — unused in registration flow but required for compilation
func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) { return nil, nil }
func (m *mockUserRepo) Update(ctx context.Context, user *domain.User) error            { return nil }

type mockPasswordHasher struct{}

func (m *mockPasswordHasher) Hash(password string) (string, error) {
	return "hashed:" + password, nil
}
func (m *mockPasswordHasher) Verify(password, encoded string) (bool, error) {
	return true, nil
}

type mockVerificationService struct {
	token string
	err   error
}

func (m *mockVerificationService) GenerateToken(ctx context.Context, email string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}
func (m *mockVerificationService) VerifyToken(ctx context.Context, token string) (*verification.TokenClaims, error) {
	return nil, nil
}

type mockTokenService struct {
	pair *token.TokenPair
	err  error
}

func (m *mockTokenService) GenerateTokenPair(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (*token.TokenPair, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pair, nil
}

type mockEventPublisher struct {
	published []publishedEvent
}

type publishedEvent struct {
	stream  string
	payload map[string]interface{}
}

func (m *mockEventPublisher) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	m.published = append(m.published, publishedEvent{stream: stream, payload: payload})
	return "msg-1", nil
}

// =============================================================================
// Test: env resolver success — env fields in event payload
// =============================================================================

func TestUseCase_ResolvesEnvDefaultsAndIncludesInEvent(t *testing.T) {
	ctx := context.Background()
	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}

	resolver := &mockResolver{
		currency:    "EUR",
		language:    "es",
		countryCode: "ES",
		timezone:    "Europe/Madrid",
	}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "verify-token-123"},
		Hasher:         &mockPasswordHasher{},
		TokenSvc: &mockTokenService{
			pair: &token.TokenPair{AccessToken: "at", RefreshToken: "rt"},
		},
		EventPublisher: publisher,
		EnvResolver:    resolver,
	})

	resp, err := uc.Execute(ctx, Command{Email: "test@example.com", Password: "password123"}, "203.0.113.42")
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
	if payload["language_code"] != "es" {
		t.Errorf("language_code = %q, want %q", payload["language_code"], "es")
	}
	if payload["currency_code"] != "EUR" {
		t.Errorf("currency_code = %q, want %q", payload["currency_code"], "EUR")
	}
	if payload["country_code"] != "ES" {
		t.Errorf("country_code = %q, want %q", payload["country_code"], "ES")
	}
	if payload["timezone_name"] != "Europe/Madrid" {
		t.Errorf("timezone_name = %q, want %q", payload["timezone_name"], "Europe/Madrid")
	}

	// The event type and stream should be correct
	if publisher.published[0].stream != "{events}:auth.user.registered" {
		t.Errorf("stream = %q, want %q", publisher.published[0].stream, "{events}:auth.user.registered")
	}
	if payload["event_type"] != "user_registered" {
		t.Errorf("event_type = %q, want %q", payload["event_type"], "user_registered")
	}
}

// =============================================================================
// Test: resolver error — env fields omitted, registration continues
// =============================================================================

func TestUseCase_ResolverError_ContinuesWithoutEnvFields(t *testing.T) {
	ctx := context.Background()
	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}

	resolver := &mockResolver{
		err: errors.New("geoip service unavailable"),
	}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "verify-token-456"},
		Hasher:         &mockPasswordHasher{},
		TokenSvc: &mockTokenService{
			pair: &token.TokenPair{AccessToken: "at", RefreshToken: "rt"},
		},
		EventPublisher: publisher,
		EnvResolver:    resolver,
	})

	_, err := uc.Execute(ctx, Command{Email: "fail@example.com", Password: "password123"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("Execute() should NOT fail on resolver error: %v", err)
	}

	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(publisher.published))
	}

	payload := publisher.published[0].payload

	// Core fields must exist
	if payload["user_id"] == nil || payload["user_id"] == "" {
		t.Error("user_id should be present")
	}
	if payload["email"] != "fail@example.com" {
		t.Errorf("email = %q, want %q", payload["email"], "fail@example.com")
	}

	// Env fields must NOT be present when resolver failed
	if _, ok := payload["language_code"]; ok {
		t.Error("language_code should NOT be present when resolver fails")
	}
	if _, ok := payload["currency_code"]; ok {
		t.Error("currency_code should NOT be present when resolver fails")
	}
	if _, ok := payload["country_code"]; ok {
		t.Error("country_code should NOT be present when resolver fails")
	}
	if _, ok := payload["timezone_name"]; ok {
		t.Error("timezone_name should NOT be present when resolver fails")
	}
}

// =============================================================================
// Test: nil resolver — no panic, reg continues
// =============================================================================

func TestUseCase_NilResolver_NoPanic(t *testing.T) {
	ctx := context.Background()
	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "verify-token-789"},
		Hasher:         &mockPasswordHasher{},
		TokenSvc: &mockTokenService{
			pair: &token.TokenPair{AccessToken: "at", RefreshToken: "rt"},
		},
		EventPublisher: publisher,
		EnvResolver:    nil, // explicitly nil
	})

	_, err := uc.Execute(ctx, Command{Email: "nilres@example.com", Password: "password123"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("Execute() should NOT panic or fail with nil resolver: %v", err)
	}

	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published event even with nil resolver, got %d", len(publisher.published))
	}

	// No env fields expected
	payload := publisher.published[0].payload
	if _, ok := payload["language_code"]; ok {
		t.Error("no env fields expected with nil resolver")
	}
}

// =============================================================================
// Test: backward compat — without envIP (empty string), resolver not called
// =============================================================================

func TestUseCase_EmptyEnvIP_ResolverNotCalled(t *testing.T) {
	ctx := context.Background()
	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}

	// This resolver will fail if called — proving it's not called
	resolver := &mockResolver{
		err: errors.New("should NOT be called"),
	}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "vt-ip-empty"},
		Hasher:         &mockPasswordHasher{},
		TokenSvc: &mockTokenService{
			pair: &token.TokenPair{AccessToken: "at", RefreshToken: "rt"},
		},
		EventPublisher: publisher,
		EnvResolver:    resolver,
	})

	// Empty IP — resolver should NOT be called
	_, err := uc.Execute(ctx, Command{Email: "emptyip@example.com", Password: "password123"}, "")
	if err != nil {
		t.Fatalf("Execute() with empty IP should succeed: %v", err)
	}

	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(publisher.published))
	}

	// No env fields when IP is empty (resolver not called)
	payload := publisher.published[0].payload
	if _, ok := payload["language_code"]; ok {
		t.Error("no env fields expected when IP is empty")
	}
}
