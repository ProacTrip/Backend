package hotel_details_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/hotel_details"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Mocks
// =============================================================================

type stubCache struct {
	getFn func(ctx context.Context, key string) (string, error)
}

func (s *stubCache) Get(ctx context.Context, key string) (string, error) {
	return s.getFn(ctx, key)
}
func (s *stubCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return nil
}

// stubHotelProvider panics via t.Error if GetHotelDetails is called — used to
// verify that rate limit guards work without calling the provider.
type stubHotelProvider struct {
	t *testing.T
}

func (s *stubHotelProvider) SearchHotels(ctx context.Context, req domain.HotelSearchRequest) (*domain.HotelSearchResponse, error) {
	return nil, nil
}

func (s *stubHotelProvider) GetHotelDetails(ctx context.Context, req domain.HotelDetailsRequest) (*domain.HotelDetailsResponse, error) {
	s.t.Error("GetHotelDetails() should NOT be called when rate limited")
	return nil, errors.New("unexpected call")
}

// =============================================================================
// Setup
// =============================================================================

func setupRateLimiter(t *testing.T) (*ratelimit.RateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	cfg := ratelimit.DefaultConfig()
	rl := ratelimit.NewRateLimiter(rdb, cfg)
	return rl, mr
}

func newTestCommand() hotel_details.Command {
	return hotel_details.Command{
		ID:          "test-property-token",
		CheckInDate: "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:      2,
		Currency:    new("USD"),
		GL:          new("us"),
		HL:          new("en"),
	}
}

// =============================================================================
// Tests
// =============================================================================

// TestExecute_RateLimitDenied verifies that when ProviderAllow returns
// Allowed=false, Execute returns domain.ErrRateLimitExceeded and does NOT
// call the provider.
func TestExecute_RateLimitDenied(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	for range 50 {
		result, err := rl.ProviderAllow(ctx, "serpapi")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if !result.Allowed {
			t.Fatal("setup failed: exhausted quota too early")
		}
	}

	provider := &stubHotelProvider{t: t}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil // cache miss
		},
	}

	uc := hotel_details.NewUseCase(hotel_details.UseCaseDeps{
		Provider:    provider,
		Cache:       cache,
		DetailsTTL:  15 * time.Minute,
		RateLimiter: rl,
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(ctx, cmd)

	if err == nil {
		t.Error("expected error when rate limited, got nil")
	}
	if !errors.Is(err, domain.ErrRateLimitExceeded) {
		t.Errorf("expected ErrRateLimitExceeded, got: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response when rate limited, got %+v", resp)
	}
}

// TestExecute_RateLimitError verifies that when ProviderAllow returns
// an error, Execute returns an internal error (5xx).
func TestExecute_RateLimitError(t *testing.T) {
	ctx := t.Context()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	cfg := &ratelimit.RateLimitConfig{
		GlobalPerMinute:        100,
		AuthenticatedPerMinute: 10,
		AnonymousPerMinute:     5,
		Providers:              map[string]ratelimit.ProviderLimit{},
	}
	rl := ratelimit.NewRateLimiter(rdb, cfg)

	provider := &stubHotelProvider{t: t}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil
		},
	}

	uc := hotel_details.NewUseCase(hotel_details.UseCaseDeps{
		Provider:    provider,
		Cache:       cache,
		DetailsTTL:  15 * time.Minute,
		RateLimiter: rl,
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(ctx, cmd)

	if err == nil {
		t.Fatal("expected error when rate limit service is unavailable, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response on rate limit error, got %+v", resp)
	}

	var prob *serrors.Problem
	if !errors.As(err, &prob) {
		t.Fatalf("expected *serrors.Problem, got %T: %v", err, err)
	}
	if prob.Status < 500 {
		t.Errorf("expected 5xx status for rate limit service failure, got %d", prob.Status)
	}
}

// =============================================================================
// Cache timestamp preservation (S-4)
// =============================================================================

// TestExecute_CacheHit_PreservesOriginalCachedAt verifies that on a cache hit,
// the usecase preserves the original CachedAt timestamp from the cache entry
// instead of overwriting it with time.Now().
func TestExecute_CacheHit_PreservesOriginalCachedAt(t *testing.T) {
	ctx := t.Context()

	originalTimestamp := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cachedResp := domain.HotelDetailsResponse{
		ID:        "test123",
		Type:      "hotel",
		Name:      "Test Hotel",
		FromCache: false, // will be set to true by usecase
		CachedAt:  &originalTimestamp,
	}
	cachedJSON, err := json.Marshal(cachedResp)
	if err != nil {
		t.Fatalf("marshal cached response: %v", err)
	}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return string(cachedJSON), nil
		},
	}

	uc := hotel_details.NewUseCase(hotel_details.UseCaseDeps{
		Provider:   &stubHotelProvider{t: t},
		Cache:      cache,
		DetailsTTL: 15 * time.Minute,
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !resp.FromCache {
		t.Error("expected FromCache=true on cache hit")
	}
	if resp.CachedAt == nil {
		t.Fatal("expected CachedAt to be non-nil")
	}
	if !resp.CachedAt.Equal(originalTimestamp) {
		t.Errorf("CachedAt was overwritten: got %v, want %v", resp.CachedAt, originalTimestamp)
	}
}
