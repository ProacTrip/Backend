// Integration tests for the flight_details handler: 4-tier default resolution.
// Verifies the handler correctly resolves GL/HL/Currency from:
//   Tier 1: explicit client params (always win)
//   Tier 2: authenticated user profile prefs (profile:{userID}:prefs Dragonfly hash)
//   Tier 3: anonymous IP environment cache (env:{ip})
//   Tier 4: config fallback
package flight_details_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/flight_details"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
)

// =============================================================================
// Handler integration test setup
// =============================================================================

// spyFlightProvider records the GetFlightDetails request for verification
// while returning a safe empty response.
type spyFlightProvider struct {
	lastReq *domain.FlightDetailsRequest
}

func (s *spyFlightProvider) SearchFlights(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
	return nil, nil
}

func (s *spyFlightProvider) GetFlightDetails(ctx context.Context, req domain.FlightDetailsRequest) (*domain.FlightDetailsResponse, error) {
	s.lastReq = &req
	return &domain.FlightDetailsResponse{}, nil
}

type noopCache struct{}

func (n *noopCache) Get(ctx context.Context, key string) (string, error) { return "", nil }
func (n *noopCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return nil
}

func setupFlightDetailsIntegrationTest(t *testing.T) (*flight_details.Handler, *spyFlightProvider, *redis.Client) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	spy := &spyFlightProvider{}

	uc := flight_details.NewUseCase(flight_details.UseCaseDeps{
		Provider:    spy,
		Cache:       &noopCache{},
		RateLimiter: nil,
		DetailsTTL:  15 * time.Minute,
	})

	defaultsCfg := shared.SearchDefaultConfig{
		Currency:    "EUR",
		Language:    "es",
		CountryCode: "AR",
	}

	handler := flight_details.NewHandler(uc, rdb, defaultsCfg)
	return handler, spy, rdb
}

func newFlightDetailsEchoContext(t *testing.T, body string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flights/details", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	return c, rec
}

// minimalFlightDetailsBody returns a JSON body that passes validation without explicit GL/HL/Currency.
func minimalFlightDetailsBody() string {
	return `{
		"booking_token": "tok_abc123",
		"departure": "EZE",
		"arrival": "MAD",
		"outbound_date": "2026-06-01"
	}`
}

// =============================================================================
// Authenticated user — profile prefs (Tier 2)
// =============================================================================

func TestHandlerIntegration_FlightDetails_AuthenticatedUser_UsesProfilePrefs(t *testing.T) {
	handler, spy, rdb := setupFlightDetailsIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Pre-populate profile prefs cache (Brazilian: BRL/pt/BR)
	profileKey := "user:prefs:" + userID.String()
	if err := rdb.HSet(ctx, profileKey, map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
		"timezone":     "America/Sao_Paulo",
	}).Err(); err != nil {
		t.Fatalf("HSet profile prefs: %v", err)
	}

	// Also pre-populate env:{ip} cache (US — should be ignored, Tier 2 wins)
	envData := map[string]interface{}{
		"location": map[string]string{
			"country_code": "US",
			"language":     "en",
			"currency":     "USD",
		},
	}
	raw, _ := json.Marshal(envData)
	rdb.Set(ctx, "env:203.0.113.42", string(raw), 0)

	c, rec := newFlightDetailsEchoContext(t, minimalFlightDetailsBody())
	c.Set("user_claims", &sharedauth.AccessClaims{
		UserID:    userID,
		Email:     "brazilian@example.com",
		RoleID:    uuid.Nil,
		SessionID: uuid.Nil,
		JTI:       uuid.Nil,
	})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the currency passed to provider comes from profile prefs
	if spy.lastReq.Currency != "BRL" {
		t.Errorf("lastReq.Currency = %q, want BRL (from profile prefs)", spy.lastReq.Currency)
	}
	// Core fields should be preserved from the request body
	if spy.lastReq.BookingToken != "tok_abc123" {
		t.Errorf("lastReq.BookingToken = %q, want tok_abc123", spy.lastReq.BookingToken)
	}
	if spy.lastReq.DepartureID != "EZE" {
		t.Errorf("lastReq.DepartureID = %q, want EZE", spy.lastReq.DepartureID)
	}
	if spy.lastReq.ArrivalID != "MAD" {
		t.Errorf("lastReq.ArrivalID = %q, want MAD", spy.lastReq.ArrivalID)
	}
}

// =============================================================================
// Anonymous user — env cache (Tier 3)
// =============================================================================

func TestHandlerIntegration_FlightDetails_AnonymousUser_UsesEnvCache(t *testing.T) {
	handler, spy, rdb := setupFlightDetailsIntegrationTest(t)
	ctx := t.Context()

	// Pre-populate env:{ip} cache for Japan (JPY/ja/JP)
	envData := map[string]interface{}{
		"location": map[string]string{
			"country_code": "JP",
			"language":     "ja",
			"currency":     "JPY",
		},
	}
	raw, err := json.Marshal(envData)
	if err != nil {
		t.Fatalf("marshal env cache: %v", err)
	}
	if err := rdb.Set(ctx, "env:8.8.8.8", string(raw), 0).Err(); err != nil {
		t.Fatalf("Set env cache: %v", err)
	}

	c, rec := newFlightDetailsEchoContext(t, minimalFlightDetailsBody())
	// Set RemoteAddr to 8.8.8.8 so c.RealIP() returns the right IP
	c.Request().RemoteAddr = "8.8.8.8:12345"
	// No user_claims — anonymous user

	err = handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify env-derived defaults (Japan: JPY)
	if spy.lastReq.Currency != "JPY" {
		t.Errorf("lastReq.Currency = %q, want JPY (from env cache)", spy.lastReq.Currency)
	}
}

// =============================================================================
// Explicit params win (Tier 1)
// =============================================================================

func TestHandlerIntegration_FlightDetails_ExplicitWinsOverProfilePrefs(t *testing.T) {
	handler, spy, rdb := setupFlightDetailsIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Pre-populate profile prefs (should be ignored because Tier 1 wins)
	rdb.HSet(ctx, "user:prefs:"+userID.String(), map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
	})

	// Request body WITH explicit currency
	body := `{"booking_token":"tok_xyz","departure":"EZE","arrival":"MAD","outbound_date":"2026-06-01","currency":"GBP"}`

	c, rec := newFlightDetailsEchoContext(t, body)
	c.Set("user_claims", &sharedauth.AccessClaims{UserID: userID})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Explicit currency must win over profile prefs
	if spy.lastReq.Currency != "GBP" {
		t.Errorf("lastReq.Currency = %q, want GBP (explicit wins)", spy.lastReq.Currency)
	}
}
