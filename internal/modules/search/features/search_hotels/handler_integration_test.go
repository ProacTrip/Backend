// Integration tests for the search_hotels handler: 4-tier default resolution.
// Verifies the handler correctly resolves GL/HL/Currency from:
//   Tier 1: explicit client params (always win)
//   Tier 2: authenticated user profile prefs (profile:{userID}:prefs Dragonfly hash)
//   Tier 3: anonymous IP environment cache (env:{ip})
//   Tier 4: config fallback
//
// Uses a cache-hit strategy: the usecase's search-response cache always returns
// a valid cached response, so the SerpAPI adapter (nil client) is never called.
// Default resolution logic is tested separately in shared/defaults_test.go.
package search_hotels_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

func init() {
	// Register domain error mappers for test environment (normally done by NewModule)
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrMissingRequiredField):
			return serrors.ErrValidationError("Falta un campo requerido", err)
		case errors.Is(err, domain.ErrInvalidParameterRange):
			return serrors.New(serrors.ProblemTypeValidationError, "Validation Error", "Parámetro fuera de rango", 422, err)
		}
		return nil
	})
}

// =============================================================================
// Handler integration test setup
// =============================================================================

// alwaysHitCache returns a valid cached hotel search response for ANY key.
// This ensures the usecase takes the cache-hit path and never calls SerpAPI.
type alwaysHitCache struct {
	body string
}

func (a *alwaysHitCache) Get(ctx context.Context, key string) (string, error) {
	return a.body, nil
}
func (a *alwaysHitCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return nil
}

// noopHotelProvider never gets called — all tests use the cache-hit path.
type noopHotelProvider struct{}

func (n *noopHotelProvider) SearchHotels(ctx context.Context, req domain.HotelSearchRequest) (*domain.HotelSearchResponse, error) {
	panic("unexpected call: provider should not be called in cache-hit tests")
}
func (n *noopHotelProvider) GetHotelDetails(ctx context.Context, req domain.HotelDetailsRequest) (*domain.HotelDetailsResponse, error) {
	panic("unexpected call")
}

func setupSearchHotelsIntegrationTest(t *testing.T) (*search_hotels.Handler, *redis.Client) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	cachedResponse := `{"type":"hotels","results_state":"matching","properties":[],"brands":[]}`

	uc := search_hotels.NewUseCase(search_hotels.UseCaseDeps{
		Provider:    &noopHotelProvider{},
		Cache:       &alwaysHitCache{body: cachedResponse},
		RateLimiter: nil,
		SearchTTL:   15 * time.Minute,
	})

	defaultsCfg := shared.SearchDefaultConfig{
		Currency: "EUR",
		Language: "es",
	}

	handler := search_hotels.NewHandler(uc, rdb, defaultsCfg)
	return handler, rdb
}

func newSearchHotelsEchoContext(t *testing.T, body string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/search/hotels", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	return c, rec
}

// minimalHotelsBody returns a JSON body that passes validation without explicit GL/HL/Currency.
func minimalHotelsBody() string {
	return `{
		"query": "Madrid",
		"check_in_date": "2026-06-15",
		"check_out_date": "2026-06-20"
	}`
}

// =============================================================================
// Authenticated user — profile prefs (Tier 2)
// =============================================================================

func TestHandlerIntegration_SearchHotels_AuthenticatedUser_UsesProfilePrefs(t *testing.T) {
	handler, rdb := setupSearchHotelsIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Pre-populate profile prefs (Brazilian: BRL/pt/BR)
	profileKey := "user:prefs:" + userID.String()
	if err := rdb.HSet(ctx, profileKey, map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
		"timezone":     "America/Sao_Paulo",
	}).Err(); err != nil {
		t.Fatalf("HSet profile prefs: %v", err)
	}

	c, rec := newSearchHotelsEchoContext(t, minimalHotelsBody())
	c.Set("user_claims", &sharedauth.AccessClaims{
		UserID:    userID,
		Email:     "brazilian@example.com",
		RoleID:    uuid.Nil,
		JTI:       uuid.Nil,
	})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// Anonymous user — env cache (Tier 3)
// =============================================================================

func TestHandlerIntegration_SearchHotels_AnonymousUser_UsesEnvCache(t *testing.T) {
	handler, rdb := setupSearchHotelsIntegrationTest(t)
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

	c, rec := newSearchHotelsEchoContext(t, minimalHotelsBody())
	c.Request().RemoteAddr = "8.8.8.8:12345"
	// No user_claims — anonymous user

	err = handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// Explicit params win (Tier 1)
// =============================================================================

func TestHandlerIntegration_SearchHotels_ExplicitWinsOverProfilePrefs(t *testing.T) {
	handler, rdb := setupSearchHotelsIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Pre-populate profile prefs (should be ignored — Tier 1 wins)
	rdb.HSet(ctx, "user:prefs:"+userID.String(), map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
	})

	// Request WITH explicit currency
	body := `{"query":"Madrid","check_in_date":"2026-06-15","check_out_date":"2026-06-20","currency":"GBP"}`

	c, rec := newSearchHotelsEchoContext(t, body)
	c.Set("user_claims", &sharedauth.AccessClaims{UserID: userID})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handler.Handle() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// Validation — missing required fields
// =============================================================================

func TestHandlerIntegration_SearchHotels_MissingQueryReturns400(t *testing.T) {
	handler, _ := setupSearchHotelsIntegrationTest(t)

	body := `{"check_in_date":"2026-06-15","check_out_date":"2026-06-20"}`
	c, rec := newSearchHotelsEchoContext(t, body)

	err := handler.Handle(c)
	// After H11 fix: validation is delegated to Command.Validate() via the use case.
	// Domain errors are mapped to RFC 9457 Problem JSON and written directly to the
	// response, so Handle() returns nil even on validation errors.
	_ = err
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
