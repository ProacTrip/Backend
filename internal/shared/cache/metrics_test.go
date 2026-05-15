// Tests unitarios para MetricsDecorator.
// Verifica contadores hit/miss/error vía expvar.
package cache

import (
	"context"
	"expvar"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// mockCache — implementación mínima de Cache para tests
// =============================================================================

type mockCache struct {
	rdb *redis.Client
}

func (m *mockCache) Get(ctx context.Context, key string) (string, error) {
	val, err := m.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // matching Dragonfly contract: "", nil on miss
	}
	return val, err
}

func (m *mockCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return m.rdb.Set(ctx, key, value, ttl).Err()
}

// =============================================================================
// Tests
// =============================================================================

func resetMetrics() {
	cacheHits.Set(0)
	cacheMisses.Set(0)
	cacheErrors.Set(0)
	cacheSets.Set(0)
	cacheSetErrors.Set(0)
}

func readIntVar(name string) int64 {
	v := expvar.Get(name)
	if v == nil {
		return 0
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

func TestMetricsDecorator_Get_Hit(t *testing.T) {
	ctx := t.Context()
	resetMetrics()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	// Pre-populate key
	requireNoErr(t, rdb.Set(ctx, "test:key", "value", 0).Err())

	inner := &mockCache{rdb: rdb}
	deco := NewMetricsDecorator(inner)

	val, err := deco.Get(ctx, "test:key")
	requireNoErr(t, err)

	if val != "value" {
		t.Errorf("expected 'value', got '%s'", val)
	}
	if got := readIntVar("cache_hits"); got != 1 {
		t.Errorf("cache_hits: expected 1, got %d", got)
	}
	if got := readIntVar("cache_misses"); got != 0 {
		t.Errorf("cache_misses: expected 0, got %d", got)
	}
	if got := readIntVar("cache_errors"); got != 0 {
		t.Errorf("cache_errors: expected 0, got %d", got)
	}
}

func TestMetricsDecorator_Get_Miss(t *testing.T) {
	ctx := t.Context()
	resetMetrics()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	inner := &mockCache{rdb: rdb}
	deco := NewMetricsDecorator(inner)

	// Key does not exist — should return "", nil (Dragonfly convention)
	val, err := deco.Get(ctx, "nonexistent")
	// mockCache.Get returns redis.Nil on miss, which MetricsDecorator converts to "", nil
	// But wait — mockCache returns "", redis.Nil. MetricsDecorator checks redis.Nil → count miss → return "", nil
	// Let me re-read the MetricsDecorator.Get impl...
	requireNoErr(t, err)

	_ = val // val is "" since mockCache returns "" on redis.Nil

	if got := readIntVar("cache_misses"); got != 1 {
		t.Errorf("cache_misses: expected 1, got %d", got)
	}
	if got := readIntVar("cache_hits"); got != 0 {
		t.Errorf("cache_hits: expected 0, got %d", got)
	}
}

func TestMetricsDecorator_Set(t *testing.T) {
	ctx := t.Context()
	resetMetrics()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	inner := &mockCache{rdb: rdb}
	deco := NewMetricsDecorator(inner)

	err := deco.Set(ctx, "test:key", "stored", 1*time.Hour)
	requireNoErr(t, err)

	if got := readIntVar("cache_sets"); got != 1 {
		t.Errorf("cache_sets: expected 1, got %d", got)
	}
	if got := readIntVar("cache_set_errors"); got != 0 {
		t.Errorf("cache_set_errors: expected 0, got %d", got)
	}

	// Verify value was actually set
	val, err := rdb.Get(ctx, "test:key").Result()
	requireNoErr(t, err)
	if val != "stored" {
		t.Errorf("expected 'stored', got '%s'", val)
	}
}

func TestMetricsDecorator_NewMetricsDecorator(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	inner := &mockCache{rdb: rdb}
	deco := NewMetricsDecorator(inner)

	if deco == nil {
		t.Fatal("expected non-nil MetricsDecorator")
	}
	if deco.inner == nil {
		t.Fatal("expected non-nil inner cache")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
