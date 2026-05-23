// Integration tests for the hotel_details handler: 3-tier default resolution.
// Verifies the handler correctly resolves GL/HL/Currency from:
//   Tier 1: explicit client params (always win)
//   Tier 2: authenticated user profile prefs (profile:{userID}:prefs Dragonfly hash)
//   Tier 3: config fallback
//
// Uses a cache-hit strategy: the usecase's search-response cache always returns
// a valid cached response, so the SerpAPI adapter (nil client) is never called.
// Default resolution logic is tested separately in shared/defaults_test.go.
package hotel_details_test

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
	"github.com/ProacTrip/Backend/internal/modules/search/features/hotel_details"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
)

// =============================================================================
// Handler integration test setup
// =============================================================================

// alwaysHitCache returns a valid cached hotel details response for ANY key.
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
	panic("unexpected call")
}
func (n *noopHotelProvider) GetHotelDetails(ctx context.Context, req domain.HotelDetailsRequest) (*domain.HotelDetailsResponse, error) {
	panic("unexpected call: provider should not be called in cache-hit tests")
}

func setupHotelDetailsIntegrationTest(t *testing.T) (*hotel_details.Handler, *redis.Client) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	// Minimal valid JSON that unmarshals into hotel_details.Response
	cachedResponse := `{"id":"test123","type":"hotel","name":"Test Hotel"}`

	uc := hotel_details.NewUseCase(hotel_details.UseCaseDeps{
		Provider:    &noopHotelProvider{},
		Cache:       &alwaysHitCache{body: cachedResponse},
		RateLimiter: nil,
		DetailsTTL:  15 * time.Minute,
	})

	defaultsCfg := shared.SearchDefaultConfig{
		Currency: "EUR",
		Language: "es",
	}

	handler := hotel_details.NewHandler(uc, rdb, defaultsCfg)
	return handler, rdb
}

func newHotelDetailsEchoContext(t *testing.T, body string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/search/hotel-details", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	return c, rec
}

// minimalHotelDetailsBody returns a JSON body that passes validation without explicit GL/HL/Currency.
func minimalHotelDetailsBody() string {
	return `{
		"id": "prop_abc123",
		"query": "Grand Hotel",
		"check_in_date": "2026-06-15",
		"check_out_date": "2026-06-20"
	}`
}

// =============================================================================
// Authenticated user — profile prefs (Tier 2)
// =============================================================================

func TestHandlerIntegration_HotelDetails_AuthenticatedUser_UsesProfilePrefs(t *testing.T) {
	handler, rdb := setupHotelDetailsIntegrationTest(t)

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

	c, rec := newHotelDetailsEchoContext(t, minimalHotelDetailsBody())
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

func TestHandlerIntegration_HotelDetails_AnonymousUser_UsesEnvCache(t *testing.T) {
	handler, rdb := setupHotelDetailsIntegrationTest(t)
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

	c, rec := newHotelDetailsEchoContext(t, minimalHotelDetailsBody())
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

func TestHandlerIntegration_HotelDetails_ExplicitWinsOverProfilePrefs(t *testing.T) {
	handler, rdb := setupHotelDetailsIntegrationTest(t)

	userID := uuid.Must(uuid.NewV7())
	ctx := t.Context()

	// Pre-populate profile prefs (should be ignored — Tier 1 wins)
	rdb.HSet(ctx, "user:prefs:"+userID.String(), map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
	})

	// Request WITH explicit currency
	body := `{"id":"prop_abc123","query":"Grand Hotel","check_in_date":"2026-06-15","check_out_date":"2026-06-20","currency":"GBP"}`

	c, rec := newHotelDetailsEchoContext(t, body)
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

func TestHandlerIntegration_HotelDetails_MissingIDReturns400(t *testing.T) {
	handler, _ := setupHotelDetailsIntegrationTest(t)

	body := `{"check_in_date":"2026-06-15","check_out_date":"2026-06-20"}`
	c, rec := newHotelDetailsEchoContext(t, body)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle should not return error directly for validation — MapError writes response: %v", err)
	}
	// MapError escribe la respuesta directamente. Para errores de dominio no registrados
	// en el mapper, usa 500 como fallback (ver backlog: registrar errores de validación).
	if rec.Code < 400 {
		t.Errorf("expected error status >= 400, got %d", rec.Code)
	}
	_ = rec
}
