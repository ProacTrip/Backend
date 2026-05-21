// Integration tests for the search_flights handler: 4-tier default resolution.
// Verifies the handler correctly resolves GL/HL/Currency from:
//   Tier 1: explicit client params (always win)
//   Tier 2: authenticated user profile prefs (profile:{userID}:prefs Dragonfly hash)
//   Tier 3: anonymous IP environment cache (env:{ip})
//   Tier 4: config fallback
package search_flights_test

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
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
)

// =============================================================================
// Handler integration test setup — creates real UseCase with stubbed deps
// =============================================================================

// spyProvider records the search request for verification while returning
// a safe empty response so the handler doesn't hit external services.
type spyProvider struct {
	lastReq *domain.FlightSearchRequest
}

func (s *spyProvider) SearchFlights(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
	s.lastReq = &req
	return &domain.FlightSearchResponse{}, nil
}
func (s *spyProvider) GetFlightDetails(ctx context.Context, req domain.FlightDetailsRequest) (*domain.FlightDetailsResponse, error) {
	return nil, nil
}

type noopCache struct{}

func (n *noopCache) Get(ctx context.Context, key string) (string, error) { return "", nil }
func (n *noopCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return nil
}

func setupHandlerIntegrationTest(t *testing.T) (*search_flights.Handler, *spyProvider, *redis.Client) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	spy := &spyProvider{}

	uc := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider:    spy,
		Cache:       &noopCache{},
		RateLimiter: nil, // nil-safe in UseCase.Execute
		SearchTTL:   15 * time.Minute,
	})

	defaultsCfg := shared.SearchDefaultConfig{
		Currency:    "EUR",
		Language:    "es",
		CountryCode: "AR",
	}

	handler := search_flights.NewHandler(uc, rdb, defaultsCfg)
	return handler, spy, rdb
}

func newEchoContext(t *testing.T, body string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/search/flights", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	return c, rec
}

// newEchoContextWithIP creates an Echo context with a known client IP via RemoteAddr.
// IP format: "1.2.3.4:port" — c.RealIP() strips the port.
func newEchoContextWithIP(t *testing.T, body string, ip string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/search/flights", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = ip + ":12345"
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	return c, rec
}

// minimalValidBody returns a JSON body that passes validation without
// explicit GL/HL/Currency.
func minimalValidBody() string {
	return `{
		"trip_type": "round_trip",
		"departure": "EZE",
		"arrival": "MAD",
		"outbound_date": "2026-06-01",
		"return_date": "2026-06-15"
	}`
}

// =============================================================================
// Task 6.2 — Integration: auth search with profile prefs (Tier 2)
// =============================================================================

// TestHandlerIntegration_AuthenticatedUser_UsesProfilePrefs verifies that when
// an authenticated user with profile prefs in cache searches without explicit
// GL/HL/Currency, the handler resolves defaults from the profile cache (Tier 2).
func TestHandlerIntegration_AuthenticatedUser_UsesProfilePrefs(t *testing.T) {
	handler, spy, rdb := setupHandlerIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Pre-populate the profile prefs cache (Brazilian user: BRL/pt/BR)
	profileKey := "user:prefs:" + userID.String()
	if err := rdb.HSet(ctx, profileKey, map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
		"timezone":     "America/Sao_Paulo",
	}).Err(); err != nil {
		t.Fatalf("HSet profile prefs: %v", err)
	}

	// Also pre-populate env:{ip} cache (US — should be ignored because Tier 2 wins)
	envData := map[string]interface{}{
		"location": map[string]string{
			"country_code": "US",
			"language":     "en",
			"currency":     "USD",
		},
	}
	raw, _ := json.Marshal(envData)
	rdb.Set(ctx, "env:203.0.113.42", string(raw), 0)

	c, rec := newEchoContext(t, minimalValidBody())

	// Set auth claims to simulate authenticated user
	c.Set("user_claims", &sharedauth.AccessClaims{
		UserID:    userID,
		Email:     "brazilian@example.com",
		RoleID:    uuid.Nil,
		SessionID: uuid.Nil,
		JTI:       uuid.Nil,
	})

	// Fire request
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the search request passed to provider has profile-derived defaults
	req := spy.lastReq
	if req == nil {
		t.Fatal("provider was never called — usecase didn't execute search")
	}

	if req.GL != "BR" {
		t.Errorf("req.GL = %q, want %q (from profile country_code)", req.GL, "BR")
	}
	if req.HL != "pt" {
		t.Errorf("req.HL = %q, want %q (from profile language)", req.HL, "pt")
	}
	if req.Currency != "BRL" {
		t.Errorf("req.Currency = %q, want %q (from profile currency)", req.Currency, "BRL")
	}

	// Core fields should be preserved from the request body
	if req.Departure != "EZE" {
		t.Errorf("req.Departure = %q, want %q", req.Departure, "EZE")
	}
	if req.Arrival != "MAD" {
		t.Errorf("req.Arrival = %q, want %q", req.Arrival, "MAD")
	}
}

// TestHandlerIntegration_ProfilePrefsBeatEnvCache verifies Tier 2 beats Tier 3.
func TestHandlerIntegration_ProfilePrefsBeatEnvCache(t *testing.T) {
	handler, spy, rdb := setupHandlerIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Tier 2: profile prefs → Argentina (ARS/es/AR)
	rdb.HSet(ctx, "user:prefs:"+userID.String(), map[string]interface{}{
		"currency":     "ARS",
		"language":     "es",
		"country_code": "AR",
	})

	// Tier 3: env cache → Japan (should be ignored — Tier 2 wins)
	envData := map[string]interface{}{
		"location": map[string]string{
			"country_code": "JP",
			"language":     "ja",
			"currency":     "JPY",
		},
	}
	raw, _ := json.Marshal(envData)
	rdb.Set(ctx, "env:1.2.3.4", string(raw), 0)

	c, rec := newEchoContext(t, minimalValidBody())
	c.Set("user_claims", &sharedauth.AccessClaims{UserID: userID})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	req := spy.lastReq
	if req == nil {
		t.Fatal("provider was never called")
	}

	// Should be ARS (Tier 2), NOT JPY (Tier 3)
	if req.Currency != "ARS" {
		t.Errorf("req.Currency = %q, want ARS (profile prefs beat env cache)", req.Currency)
	}
	if req.GL != "AR" {
		t.Errorf("req.GL = %q, want AR", req.GL)
	}
	if req.HL != "es" {
		t.Errorf("req.HL = %q, want es", req.HL)
	}
}

// =============================================================================
// Task 6.3 — Integration: anonymous search with IP env cache (Tier 3)
// =============================================================================

// TestHandlerIntegration_AnonymousUser_UsesEnvCache verifies that anonymous
// users (no auth claims) get defaults from the IP environment cache (Tier 3).
func TestHandlerIntegration_AnonymousUser_UsesEnvCache(t *testing.T) {
	handler, spy, rdb := setupHandlerIntegrationTest(t)
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

	// Use RemoteAddr to set the client IP (env:{8.8.8.8} cache)
	c, rec := newEchoContextWithIP(t, minimalValidBody(), "8.8.8.8")
	// No user_claims set — anonymous user

	err = handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req := spy.lastReq
	if req == nil {
		t.Fatal("provider was never called")
	}

	// Verify env-derived defaults (Japan: JPY/ja/JP)
	if req.GL != "JP" {
		t.Errorf("req.GL = %q, want %q (from env cache country_code)", req.GL, "JP")
	}
	if req.HL != "ja" {
		t.Errorf("req.HL = %q, want %q (from env cache language)", req.HL, "ja")
	}
	if req.Currency != "JPY" {
		t.Errorf("req.Currency = %q, want %q (from env cache currency)", req.Currency, "JPY")
	}

	// Core request fields should be preserved
	if req.Departure != "EZE" {
		t.Errorf("req.Departure = %q, want %q", req.Departure, "EZE")
	}
	if req.Arrival != "MAD" {
		t.Errorf("req.Arrival = %q, want %q", req.Arrival, "MAD")
	}
}

// TestHandlerIntegration_EnvCacheMiss_FallsToConfig verifies Tier 3 miss → Tier 4.
func TestHandlerIntegration_EnvCacheMiss_FallsToConfig(t *testing.T) {
	handler, spy, _ := setupHandlerIntegrationTest(t)

	c, rec := newEchoContext(t, minimalValidBody())
	// No profile cache, no env cache, no auth — fallback to config defaults

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	req := spy.lastReq
	if req == nil {
		t.Fatal("provider was never called")
	}

	// Config defaults are AR/es/EUR (see setupHandlerIntegrationTest)
	if req.GL != "AR" {
		t.Errorf("req.GL = %q, want AR (config default)", req.GL)
	}
	if req.HL != "es" {
		t.Errorf("req.HL = %q, want es (config default)", req.HL)
	}
	if req.Currency != "EUR" {
		t.Errorf("req.Currency = %q, want EUR (config default)", req.Currency)
	}
}

// =============================================================================
// Tier 1 — Explicit params always win over all other tiers
// =============================================================================

// TestHandlerIntegration_ExplicitWinsOverProfilePrefs verifies Tier 1 beats Tier 2.
func TestHandlerIntegration_ExplicitWinsOverProfilePrefs(t *testing.T) {
	handler, spy, rdb := setupHandlerIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Pre-populate profile prefs (should be ignored because Tier 1 wins)
	rdb.HSet(ctx, "user:prefs:"+userID.String(), map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
	})

	// Request body WITH explicit GL/HL/Currency
	body := `{"trip_type":"round_trip","departure":"EZE","arrival":"MAD","outbound_date":"2026-06-01","return_date":"2026-06-15","gl":"GB","hl":"en","currency":"GBP"}`

	c, rec := newEchoContext(t, body)
	c.Set("user_claims", &sharedauth.AccessClaims{UserID: userID})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	req := spy.lastReq
	if req == nil {
		t.Fatal("provider was never called")
	}

	// Explicit values from body must win over profile prefs
	if req.GL != "GB" {
		t.Errorf("req.GL = %q, want GB (explicit)", req.GL)
	}
	if req.HL != "en" {
		t.Errorf("req.HL = %q, want en (explicit)", req.HL)
	}
	if req.Currency != "GBP" {
		t.Errorf("req.Currency = %q, want GBP (explicit)", req.Currency)
	}
}

// TestHandlerIntegration_SingleExplicitWins verifies:
// if ANY explicit param is set, Tier 1 wins. Non-explicit params remain nil.
func TestHandlerIntegration_SingleExplicitWins(t *testing.T) {
	handler, spy, rdb := setupHandlerIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Pre-populate profile prefs
	rdb.HSet(ctx, "user:prefs:"+userID.String(), map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
	})

	// Only currency is explicit — GL and HL not provided
	body := `{"trip_type":"round_trip","departure":"EZE","arrival":"MAD","outbound_date":"2026-06-01","return_date":"2026-06-15","currency":"GBP"}`

	c, rec := newEchoContext(t, body)
	c.Set("user_claims", &sharedauth.AccessClaims{UserID: userID})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	req := spy.lastReq
	if req == nil {
		t.Fatal("provider was never called")
	}

	// Currency must be explicit
	if req.Currency != "GBP" {
		t.Errorf("req.Currency = %q, want GBP (explicit)", req.Currency)
	}

	// GL/HL should be empty because Tier 1 shortcut skips resolution entirely
	// When ANY explicit param is present, ResolveSearchDefaults returns empty
	// for non-explicit params (ptrOrEmpty returns "" for nil pointers)
	if req.GL != "" {
		t.Errorf("req.GL = %q, want empty (Tier 1 wins, non-explicit params left empty)", req.GL)
	}
	if req.HL != "" {
		t.Errorf("req.HL = %q, want empty (Tier 1 wins, non-explicit params left empty)", req.HL)
	}
}
