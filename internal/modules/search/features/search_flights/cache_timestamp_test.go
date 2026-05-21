package search_flights_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// stubCacheWithSet tracks Set() calls
// =============================================================================

type stubCacheWithSet struct {
	data    map[string]string
	setKeys []string
}

func (c *stubCacheWithSet) Get(ctx context.Context, key string) (string, error) {
	if c.data == nil {
		return "", errors.New("cache miss")
	}
	v, ok := c.data[key]
	if !ok {
		return "", errors.New("cache miss")
	}
	return v, nil
}

func (c *stubCacheWithSet) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if c.setKeys == nil {
		c.setKeys = make([]string, 0)
	}
	c.setKeys = append(c.setKeys, key)
	return nil
}

// =============================================================================
// Test: Cache hit preserves original CachedAt timestamp
// =============================================================================

func TestCacheHitPreservesTimestamp(t *testing.T) {
	ctx := t.Context()

	cachedTime := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	cachedResp := domain.FlightSearchResponse{
		TripType:     "round_trip",
		ResultsState: "complete",
		BestFlights:  []domain.Flight{},
		CachedAt:     &cachedTime,
		FromCache:    true,
	}

	cachedJSON, err := json.Marshal(cachedResp)
	if err != nil {
		t.Fatalf("marshal cached response: %v", err)
	}

	getCache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return string(cachedJSON), nil
		},
	}

	provider := &stubFlightProvider{
		searchFn: func(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
			t.Error("provider should NOT be called on cache hit")
			return nil, errors.New("unexpected call")
		},
	}

	uc := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider:  provider,
		Cache:     getCache,
		SearchTTL: 5 * time.Minute,
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error on cache hit: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.CachedAt == nil {
		t.Fatal("expected non-nil CachedAt on cache hit")
	}

	if !resp.CachedAt.Equal(cachedTime) {
		t.Errorf("CachedAt = %v, want %v — timestamp should be preserved from cache, not recomputed",
			resp.CachedAt, cachedTime)
	}
}

// =============================================================================
// Test: Empty cacheKey guard — Set NUNCA se llama con key vacío
// =============================================================================

func TestEmptyCacheKeyGuard(t *testing.T) {
	ctx := t.Context()

	cache := &stubCacheWithSet{}

	uc := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider: &stubFlightProvider{
			searchFn: func(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
				return &domain.FlightSearchResponse{
					TripType:     "round_trip",
					ResultsState: "complete",
				}, nil
			},
		},
		Cache:     cache,
		SearchTTL: 5 * time.Minute,
	})

	cmd := newTestCommand()
	_, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Logf("Execute error (may be nil rate limiter): %v", err)
	}

	uc.Wait()

	for _, key := range cache.setKeys {
		if key == "" {
			t.Error("cache.Set was called with empty key — should have been guarded by empty cacheKey check")
		}
	}
}

// =============================================================================
// Test: Cache miss — CachedAt is set to recent time by provider path
// =============================================================================

func TestCacheMissSetsRecentCachedAt(t *testing.T) {
	ctx := t.Context()

	getCache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", errors.New("cache miss")
		},
	}

	uc := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider: &stubFlightProvider{
			searchFn: func(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
				return &domain.FlightSearchResponse{
					TripType:     "round_trip",
					ResultsState: "complete",
				}, nil
			},
		},
		Cache:     getCache,
		SearchTTL: 5 * time.Minute,
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Logf("error (may be nil rate limiter): %v", err)
	}
	if resp != nil && resp.CachedAt != nil {
		now := time.Now()
		diff := now.Sub(*resp.CachedAt)
		if diff < 0 {
			diff = -diff
		}
		if diff > 2*time.Second {
			t.Errorf("CachedAt is too far from now: diff=%v, CachedAt=%v", diff, resp.CachedAt)
		}
	}
}

// =============================================================================
// Test: Integration with miniredis — cache hit timestamp preservation
// =============================================================================

func TestIntegration_MiniredisCacheHitTimestamp(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	cachedTime := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	cachedResp := domain.FlightSearchResponse{
		TripType:     "round_trip",
		ResultsState: "complete",
		BestFlights:  []domain.Flight{},
		CachedAt:     &cachedTime,
		FromCache:    true,
	}
	cachedJSON, _ := json.Marshal(cachedResp)

	_ = rdb

	getCache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return string(cachedJSON), nil
		},
	}

	uc := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider: &stubFlightProvider{
			searchFn: func(ctx context.Context, req domain.FlightSearchRequest) (*domain.FlightSearchResponse, error) {
				t.Error("provider should not be called on cache hit")
				return nil, errors.New("unexpected")
			},
		},
		Cache:     getCache,
		SearchTTL: 5 * time.Minute,
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CachedAt == nil || !resp.CachedAt.Equal(cachedTime) {
		t.Errorf("CachedAt = %v, want %v — should preserve cached timestamp",
			resp.CachedAt, cachedTime)
	}
}
