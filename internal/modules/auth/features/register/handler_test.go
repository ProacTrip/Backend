package register

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
)

// =============================================================================
// Test: handler captures IP from c.RealIP() and passes it to usecase.Execute()
// Verifies that envIP reaches the EnvironmentResolver.ResolveDefaults call.
// =============================================================================

func TestHandler_PassesRealIPToUseCase(t *testing.T) {
	e := echo.New()

	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}

	// resolverSpy records the IP it received — proving the handler passed it correctly
	resolverSpy := &resolverSpy{
		currency:    "ARS",
		language:    "es",
		countryCode: "AR",
		timezone:    "America/Argentina/Buenos_Aires",
	}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "vt-realip"},
		Hasher:         &mockPasswordHasher{},
		TokenSvc:       &mockTokenService{pair: &token.TokenPair{AccessToken: "at", RefreshToken: "rt"}},
		EventPublisher: publisher,
		EnvResolver:    resolverSpy,
	})

	handler := NewHandler(uc)

	body := `{"email":"realip@test.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "203.0.113.42:54321"

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	// Verify the resolver received the correct client IP
	if !resolverSpy.called {
		t.Fatal("EnvironmentResolver.ResolveDefaults was not called")
	}
	if resolverSpy.lastIP != "203.0.113.42" {
		t.Errorf("resolver received IP %q, want %q", resolverSpy.lastIP, "203.0.113.42")
	}

	// Verify env fields are in the published event payload
	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(publisher.published))
	}
	payload := publisher.published[0].payload
	if payload["language_code"] != "es" {
		t.Errorf("language_code = %q, want %q", payload["language_code"], "es")
	}
	if payload["currency_code"] != "ARS" {
		t.Errorf("currency_code = %q, want %q", payload["currency_code"], "ARS")
	}
	if payload["country_code"] != "AR" {
		t.Errorf("country_code = %q, want %q", payload["country_code"], "AR")
	}
	if payload["timezone_name"] != "America/Argentina/Buenos_Aires" {
		t.Errorf("timezone_name = %q, want %q", payload["timezone_name"], "America/Argentina/Buenos_Aires")
	}

	if rec.Code != http.StatusCreated {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusCreated)
	}
}

// =============================================================================
// Test: handler with missing body returns validation error
// (regression — IP capture doesn't break existing validation flow)
// =============================================================================

func TestHandler_MissingBody_ReturnsError(t *testing.T) {
	e := echo.New()

	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}
	resolverSpy := &resolverSpy{}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "vt"},
		Hasher:         &mockPasswordHasher{},
		TokenSvc:       &mockTokenService{pair: &token.TokenPair{AccessToken: "at", RefreshToken: "rt"}},
		EventPublisher: publisher,
		EnvResolver:    resolverSpy,
	})

	handler := NewHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/register", nil)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "198.51.100.1:12345"

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Handle(c)

	// MapError handles the error internally (writes to response, returns nil)
	// The handler returns nil but the response has the error status code
	if err != nil {
		t.Logf("Handle returned error (also valid): %v", err)
	}

	// Verify the response status code is a client error (4xx)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Errorf("expected 4xx status for validation error, got %d", rec.Code)
	}
}

// =============================================================================
// Test: handler with IPv6 address
// =============================================================================

func TestHandler_IPv6Address_PassedCorrectly(t *testing.T) {
	e := echo.New()

	repo := newMockUserRepo()
	publisher := &mockEventPublisher{}
	resolverSpy := &resolverSpy{
		currency:    "EUR",
		language:    "en",
		countryCode: "DE",
		timezone:    "Europe/Berlin",
	}

	uc := NewUseCase(UseCaseDeps{
		Repo:           repo,
		VerifySvc:      &mockVerificationService{token: "vt-ipv6"},
		Hasher:         &mockPasswordHasher{},
		TokenSvc:       &mockTokenService{pair: &token.TokenPair{AccessToken: "at", RefreshToken: "rt"}},
		EventPublisher: publisher,
		EnvResolver:    resolverSpy,
	})

	handler := NewHandler(uc)

	body := `{"email":"ipv6@test.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "[2001:db8::1]:54321"

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	if !resolverSpy.called {
		t.Fatal("EnvironmentResolver.ResolveDefaults was not called")
	}
	if resolverSpy.lastIP != "2001:db8::1" {
		t.Errorf("resolver received IP %q, want %q", resolverSpy.lastIP, "2001:db8::1")
	}
}

// =============================================================================
// resolverSpy — mock EnvironmentResolver that records the IP argument
// =============================================================================

type resolverSpy struct {
	called      bool
	lastIP      string
	currency    string
	language    string
	countryCode string
	timezone    string
	err         error
}

func (r *resolverSpy) ResolveDefaults(ctx context.Context, ip string) (currency, language, countryCode, timezone string, err error) {
	r.called = true
	r.lastIP = ip
	if r.err != nil {
		return "", "", "", "", r.err
	}
	return r.currency, r.language, r.countryCode, r.timezone, nil
}

// compile-time interface check
var _ EnvironmentResolver = (*resolverSpy)(nil)
