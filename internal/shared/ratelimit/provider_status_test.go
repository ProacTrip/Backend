package ratelimit

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestProviderStatusPeeksWithoutConsuming(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	cfg := &RateLimitConfig{
		Providers: map[string]ProviderLimit{
			"serpapi": {MaxRequests: 10, Window: time.Hour},
		},
	}
	rl := NewRateLimiter(rdb, cfg)

	// Peek before any consumption — debería mostrar limit=10, remaining=10
	status, err := rl.ProviderStatus(t.Context(), "serpapi")
	if err != nil {
		t.Fatalf("ProviderStatus failed: %v", err)
	}
	if status.Limit != 10 {
		t.Errorf("Limit = %d, want 10", status.Limit)
	}
	if status.Remaining != 10 {
		t.Errorf("Remaining = %d, want 10", status.Remaining)
	}
	if status.Current != 0 {
		t.Errorf("Current = %d, want 0", status.Current)
	}

	// Consumir 3 tokens via ProviderAllow
	for range 3 {
		if _, err := rl.ProviderAllow(t.Context(), "serpapi"); err != nil {
			t.Fatalf("ProviderAllow failed: %v", err)
		}
	}

	// Peek después de consumo — remaining debe ser 7, no 6
	status, err = rl.ProviderStatus(t.Context(), "serpapi")
	if err != nil {
		t.Fatalf("ProviderStatus after consume failed: %v", err)
	}
	if status.Remaining != 7 {
		t.Errorf("Remaining after 3 consumes = %d, want 7 (peek no consume)", status.Remaining)
	}
	if status.Current != 3 {
		t.Errorf("Current after 3 consumes = %d, want 3", status.Current)
	}

	// Verificar que ProviderStatus no consume: remaining sigue siendo 7
	status2, err := rl.ProviderStatus(t.Context(), "serpapi")
	if err != nil {
		t.Fatalf("second ProviderStatus failed: %v", err)
	}
	if status2.Remaining != 7 {
		t.Errorf("Remaining after double peek = %d, want 7 (peek no consume)", status2.Remaining)
	}
}

func TestProviderStatusUnknownProvider(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	cfg := &RateLimitConfig{
		Providers: map[string]ProviderLimit{
			"serpapi": {MaxRequests: 10, Window: time.Hour},
		},
	}
	rl := NewRateLimiter(rdb, cfg)

	_, err := rl.ProviderStatus(t.Context(), "unknown")
	if err == nil {
		t.Error("ProviderStatus for unknown provider should return error")
	}
}

func TestProviderStatusReturnsResetTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	cfg := &RateLimitConfig{
		Providers: map[string]ProviderLimit{
			"serpapi": {MaxRequests: 10, Window: time.Hour},
		},
	}
	rl := NewRateLimiter(rdb, cfg)

	// Consumir para crear la key
	rl.ProviderAllow(t.Context(), "serpapi")

	status, err := rl.ProviderStatus(t.Context(), "serpapi")
	if err != nil {
		t.Fatalf("ProviderStatus failed: %v", err)
	}
	if status.ResetTTL <= 0 {
		t.Errorf("ResetTTL = %v, should be > 0", status.ResetTTL)
	}
}
