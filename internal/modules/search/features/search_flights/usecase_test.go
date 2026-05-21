package search_flights_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Mocks for search_flights
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

type stubFlightProvider struct {
	searchFn func(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error)
}

func (s *stubFlightProvider) SearchFlights(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
	return s.searchFn(ctx, req)
}
func (s *stubFlightProvider) GetFlightDetails(ctx context.Context, req domain.FlightDetailsRequest) (*domain.FlightDetailsResponse, error) {
	return nil, nil
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

func newTestCommand() search_flights.Command {
	return search_flights.Command{
		TripType:     search_flights.TripTypeRoundTrip,
		Departure:    "EZE",
		Arrival:      "MAD",
		OutboundDate: "2026-06-15",
		ReturnDate:   "2026-06-30",
		Adults:       1,
		Currency:     new("USD"),
		Limit:        10,
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

	// Exhaust serpapi quota: default is 50 per hour.
	// We call ProviderAllow 50 times to reach the limit.
	for range 50 {
		result, err := rl.ProviderAllow(ctx, "serpapi")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if !result.Allowed {
			t.Fatal("setup failed: exhausted quota too early")
		}
	}

	// Now the next call should be denied.
	result, err := rl.ProviderAllow(ctx, "serpapi")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if result.Allowed {
		t.Fatal("setup failed: expected quota to be exhausted at request 51")
	}

	provider := &stubFlightProvider{
		searchFn: func(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
			t.Error("provider.SearchFlights() should NOT be called when rate limited")
			return nil, errors.New("unexpected call")
		},
	}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil // cache miss
		},
	}

	uc := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider:    provider,
		Cache:       cache,
		SearchTTL:   15 * time.Minute,
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
// an error (Redis unavailable / unknown provider), Execute returns an
// internal error and does NOT call the provider.
func TestExecute_RateLimitError(t *testing.T) {
	ctx := t.Context()

	// Create a RateLimiter without "serpapi" in the config → ProviderAllow fails.
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	cfg := &ratelimit.RateLimitConfig{
		GlobalPerMinute:        100,
		AuthenticatedPerMinute: 10,
		AnonymousPerMinute:     5,
		Providers:              map[string]ratelimit.ProviderLimit{}, // empty — no serpapi
	}
	rl := ratelimit.NewRateLimiter(rdb, cfg)

	provider := &stubFlightProvider{
		searchFn: func(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
			t.Error("provider.SearchFlights() should NOT be called when rate limit service is down")
			return nil, errors.New("unexpected call")
		},
	}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil // cache miss
		},
	}

	uc := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider:    provider,
		Cache:       cache,
		SearchTTL:   15 * time.Minute,
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

	// Verify it's an internal server error (500), not a 429
	var prob *serrors.Problem
	if !errors.As(err, &prob) {
		t.Fatalf("expected *serrors.Problem, got %T: %v", err, err)
	}
	if prob.Status < 500 {
		t.Errorf("expected 5xx status for rate limit service failure, got %d", prob.Status)
	}
}
