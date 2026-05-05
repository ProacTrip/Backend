package flight_details_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/flight_details"
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

type stubFlightProvider struct {
	getDetailsFn func(ctx context.Context, req domain.FlightDetailsRequest) (*domain.FlightDetailsResponse, error)
}

func (s *stubFlightProvider) SearchFlights(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
	return nil, nil
}
func (s *stubFlightProvider) GetFlightDetails(ctx context.Context, req domain.FlightDetailsRequest) (*domain.FlightDetailsResponse, error) {
	return s.getDetailsFn(ctx, req)
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

func newTestCommand() flight_details.Command {
	return flight_details.Command{
		BookingToken: "test-token-123",
		Adults:       1,
		Currency:     new("USD"),
		Departure:    "EZE",
		Arrival:      "MAD",
		OutboundDate: "2026-06-15",
		ReturnDate:   "2026-06-30",
	}
}

// =============================================================================
// Tests
// =============================================================================

// TestExecute_RateLimitDenied verifies that when ProviderAllow returns
// Allowed=false, Execute returns domain.ErrRateLimitExceeded.
func TestExecute_RateLimitDenied(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	// Exhaust serpapi quota
	for range 50 {
		result, err := rl.ProviderAllow(ctx, "serpapi")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if !result.Allowed {
			t.Fatal("setup failed: exhausted quota too early")
		}
	}

	provider := &stubFlightProvider{
		getDetailsFn: func(ctx context.Context, req domain.FlightDetailsRequest) (*domain.FlightDetailsResponse, error) {
			t.Error("provider.GetFlightDetails() should NOT be called when rate limited")
			return nil, errors.New("unexpected call")
		},
	}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil // cache miss
		},
	}

	uc := flight_details.NewUseCase(flight_details.UseCaseDeps{
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

	provider := &stubFlightProvider{
		getDetailsFn: func(ctx context.Context, req domain.FlightDetailsRequest) (*domain.FlightDetailsResponse, error) {
			t.Error("provider.GetFlightDetails() should NOT be called when rate limit service is down")
			return nil, errors.New("unexpected call")
		},
	}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil
		},
	}

	uc := flight_details.NewUseCase(flight_details.UseCaseDeps{
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
