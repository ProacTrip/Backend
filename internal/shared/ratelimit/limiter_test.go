package ratelimit_test

import (
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupRateLimiter(t *testing.T) (*ratelimit.RateLimiter, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { rdb.Close() })

	cfg := ratelimit.DefaultConfig()
	rl := ratelimit.NewRateLimiter(rdb, cfg)
	return rl, mr
}

func TestRateLimiterAllow(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	result, err := rl.Allow(ctx, "test:ip:1.2.3.4", 3, 60)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("first request should be allowed")
	}
	if result.Current != 1 {
		t.Errorf("current = %d, want 1", result.Current)
	}
	if result.Remaining != 2 {
		t.Errorf("remaining = %d, want 2", result.Remaining)
	}
	if result.Limit != 3 {
		t.Errorf("limit = %d, want 3", result.Limit)
	}
}

func TestRateLimiterExceeded(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	for i := range 3 {
		result, err := rl.Allow(ctx, "test:ip:exceed", 3, 60)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !result.Allowed {
			t.Errorf("request %d should be allowed, current=%d", i, result.Current)
		}
	}

	result, err := rl.Allow(ctx, "test:ip:exceed", 3, 60)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("4th request should NOT be allowed")
	}
	if result.Current != 4 {
		t.Errorf("current = %d, want 4", result.Current)
	}
	if result.Remaining != 0 {
		t.Errorf("remaining = %d, want 0", result.Remaining)
	}
}

func TestRateLimiterTTLSet(t *testing.T) {
	ctx := t.Context()
	rl, mr := setupRateLimiter(t)

	key := ratelimit.HashtagRateLimit + ":test:ip:ttl"
	result, err := rl.Allow(ctx, "test:ip:ttl", 1, 60)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("first request should be allowed")
	}

	ttl := mr.TTL(key)
	if ttl <= 0*time.Second {
		t.Errorf("TTL should be set, got %v", ttl)
	}
	if ttl > 61*time.Second {
		t.Errorf("TTL too long: %v", ttl)
	}
}

func TestRateLimiterTTLNotResetOnSubsequentCalls(t *testing.T) {
	ctx := t.Context()
	rl, mr := setupRateLimiter(t)

	key := ratelimit.HashtagRateLimit + ":test:ip:ttl-multi"
	rl.Allow(ctx, "test:ip:ttl-multi", 10, 60)

	initialTTL := mr.TTL(key)

	rl.Allow(ctx, "test:ip:ttl-multi", 10, 60)

	finalTTL := mr.TTL(key)

	ttlDiff := initialTTL - finalTTL
	if ttlDiff > 2*time.Second || ttlDiff < 0 {
		t.Errorf("TTL should decrease by ~1s, initial=%v, final=%v, diff=%v", initialTTL, finalTTL, ttlDiff)
	}
}

func TestGlobalAllow(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	result, err := rl.GlobalAllow(ctx, "192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("first global request should be allowed")
	}
	if result.Limit != 100 {
		t.Errorf("global limit = %d, want 100", result.Limit)
	}
}

func TestAuthenticatedAllow(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	result, err := rl.AuthenticatedAllow(ctx, "user-uuid-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("first authenticated request should be allowed")
	}
	if result.Limit != 10 {
		t.Errorf("auth limit = %d, want 10", result.Limit)
	}
}

func TestAnonymousAllow(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	result, err := rl.AnonymousAllow(ctx, "anon-cookie-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("first anonymous request should be allowed")
	}
	if result.Limit != 5 {
		t.Errorf("anon limit = %d, want 5", result.Limit)
	}
}

func TestProviderAllowSerpAPI(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	result, err := rl.ProviderAllow(ctx, "serpapi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("first serpapi request should be allowed")
	}
	if result.Limit != 50 {
		t.Errorf("serpapi limit = %d, want 50", result.Limit)
	}
}

func TestProviderAllowResend(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	result, err := rl.ProviderAllow(ctx, "resend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("first resend request should be allowed")
	}
	if result.Limit != 100 {
		t.Errorf("resend limit = %d, want 100", result.Limit)
	}
}

func TestProviderAllowUnknown(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	result, err := rl.ProviderAllow(ctx, "unknown")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
	if result.Allowed {
		t.Error("expected Allowed=false for unknown provider (fail-closed)")
	}
}

func TestProviderAllowUnknownBlocked(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	result, err := rl.ProviderAllow(ctx, "serpai")
	if err == nil {
		t.Error("expected error for typo provider 'serpai'")
	}
	if result.Allowed {
		t.Error("expected Allowed=false for typo provider (fail-closed)")
	}
}

func TestRateLimiterIndependentKeys(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	for range 5 {
		_, err := rl.Allow(ctx, "test:ip:key-a", 5, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	result, err := rl.Allow(ctx, "test:ip:key-b", 5, 60)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("key-b should be allowed (independent from key-a)")
	}
	if result.Current != 1 {
		t.Errorf("key-b current = %d, want 1", result.Current)
	}
}

func TestRateLimitConfigDefaults(t *testing.T) {
	cfg := ratelimit.DefaultConfig()

	if cfg.GlobalPerMinute != 100 {
		t.Errorf("GlobalPerMinute = %d, want 100", cfg.GlobalPerMinute)
	}
	if cfg.AuthenticatedPerMinute != 10 {
		t.Errorf("AuthenticatedPerMinute = %d, want 10", cfg.AuthenticatedPerMinute)
	}
	if cfg.AnonymousPerMinute != 5 {
		t.Errorf("AnonymousPerMinute = %d, want 5", cfg.AnonymousPerMinute)
	}

	serpapi, ok := cfg.Providers["serpapi"]
	if !ok {
		t.Fatal("serpapi provider not configured")
	}
	if serpapi.MaxRequests != 50 {
		t.Errorf("serpapi MaxRequests = %d, want 50", serpapi.MaxRequests)
	}
	if serpapi.Window != 1*time.Hour {
		t.Errorf("serpapi Window = %v, want 1h", serpapi.Window)
	}

	resend, ok := cfg.Providers["resend"]
	if !ok {
		t.Fatal("resend provider not configured")
	}
	if resend.MaxRequests != 100 {
		t.Errorf("resend MaxRequests = %d, want 100", resend.MaxRequests)
	}
	if resend.Window != 24*time.Hour {
		t.Errorf("resend Window = %v, want 24h", resend.Window)
	}
}

func TestRateLimitConfigDynamicProvider(t *testing.T) {
	t.Setenv("RATELIMIT_PROVIDER_AI_MAX", "20")
	t.Setenv("RATELIMIT_PROVIDER_AI_WINDOW_SEC", "3600")

	cfg := ratelimit.LoadRateLimitConfig()

	aiProvider, ok := cfg.Providers["ai"]
	if !ok {
		t.Fatal(`expected "ai" provider in cfg.Providers — dynamic env var detection failed`)
	}
	if aiProvider.MaxRequests != 20 {
		t.Errorf("ai MaxRequests = %d, want 20", aiProvider.MaxRequests)
	}
	if aiProvider.Window != 1*time.Hour {
		t.Errorf("ai Window = %v, want 1h", aiProvider.Window)
	}
}

func TestRateLimitConfigDynamicProviderStripe(t *testing.T) {
	t.Setenv("RATELIMIT_PROVIDER_STRIPE_MAX", "500")
	t.Setenv("RATELIMIT_PROVIDER_STRIPE_WINDOW_SEC", "60")

	cfg := ratelimit.LoadRateLimitConfig()

	stripeProvider, ok := cfg.Providers["stripe"]
	if !ok {
		t.Fatal(`expected "stripe" provider in cfg.Providers — dynamic env var detection failed`)
	}
	if stripeProvider.MaxRequests != 500 {
		t.Errorf("stripe MaxRequests = %d, want 500", stripeProvider.MaxRequests)
	}
	if stripeProvider.Window != 60*time.Second {
		t.Errorf("stripe Window = %v, want 60s", stripeProvider.Window)
	}
}
