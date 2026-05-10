// Rate limiter atómico usando Redis con Lua script.
// Evita race conditions usando INCR + EXPIRE atómicos.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const HashtagRateLimit = "{ratelimit}"

var rateLimitScript = redis.NewScript(`
	local current = redis.call('INCR', KEYS[1])
	if current == 1 then
		redis.call('EXPIRE', KEYS[1], ARGV[1])
	end
	local ttl = redis.call('TTL', KEYS[1])
	return {current, ttl}
`)

type RateLimitResult struct {
	Allowed   bool
	Current   int64
	Limit     int64
	Remaining int64
	ResetTTL  time.Duration
}

// RateLimiter ejecuta el script Lua en Redis para verificar límites
type RateLimiter struct {
	rdb    *redis.Client
	script *redis.Script
	cfg    *RateLimitConfig
}

func NewRateLimiter(rdb *redis.Client, cfg *RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		rdb:    rdb,
		script: rateLimitScript,
		cfg:    cfg,
	}
}

func (rl *RateLimiter) Config() *RateLimitConfig {
	return rl.cfg
}

func (rl *RateLimiter) Allow(ctx context.Context, key string, maxRequests int, windowSecs int) (RateLimitResult, error) {
	key = fmt.Sprintf("%s:%s", HashtagRateLimit, key)

	result := rl.script.Run(ctx, rl.rdb, []string{key}, windowSecs)
	values, err := result.Slice()
	if err != nil {
		return RateLimitResult{}, fmt.Errorf("rate limit script: %w", err)
	}

	if len(values) != 2 {
		return RateLimitResult{}, fmt.Errorf("rate limit script: unexpected result length %d", len(values))
	}

	current, ok := values[0].(int64)
	if !ok {
		return RateLimitResult{}, fmt.Errorf("unexpected rate limit result type for current: %T", values[0])
	}
	ttlSec, ok := values[1].(int64)
	if !ok {
		return RateLimitResult{}, fmt.Errorf("unexpected rate limit result type for ttl: %T", values[1])
	}

	limit := int64(maxRequests)
	remaining := limit - current
	if remaining < 0 {
		remaining = 0
	}
	resetTTL := time.Duration(max(ttlSec, 0)) * time.Second

	return RateLimitResult{
		Allowed:   current <= limit,
		Current:   current,
		Limit:     limit,
		Remaining: remaining,
		ResetTTL:  resetTTL,
	}, nil
}

func (rl *RateLimiter) AllowWindow(ctx context.Context, key string, maxRequests int, window time.Duration) (RateLimitResult, error) {
	return rl.Allow(ctx, key, maxRequests, int(window.Seconds()))
}

func (rl *RateLimiter) GlobalAllow(ctx context.Context, ip string) (RateLimitResult, error) {
	return rl.Allow(ctx, "global:"+ip, rl.cfg.GlobalPerMinute, 60)
}

func (rl *RateLimiter) AuthenticatedAllow(ctx context.Context, userID string) (RateLimitResult, error) {
	return rl.Allow(ctx, "auth:"+userID, rl.cfg.AuthenticatedPerMinute, 60)
}

func (rl *RateLimiter) AnonymousAllow(ctx context.Context, cookieID string) (RateLimitResult, error) {
	return rl.Allow(ctx, "anon:"+cookieID, rl.cfg.AnonymousPerMinute, 60)
}

func (rl *RateLimiter) ProviderAllow(ctx context.Context, provider string) (RateLimitResult, error) {
	pl, ok := rl.cfg.Providers[provider]
	if !ok {
		return RateLimitResult{Allowed: false}, fmt.Errorf("unknown provider: %s", provider)
	}

	var windowKey string
	switch pl.Window {
	case 24 * time.Hour:
		windowKey = time.Now().UTC().Format("2006-01-02")
	case 1 * time.Hour:
		windowKey = time.Now().UTC().Format("2006-01-02T15")
	default:
		windowKey = fmt.Sprintf("%d", time.Now().UTC().Unix()/int64(pl.Window.Seconds()))
	}

	key := fmt.Sprintf("provider:%s:%s", provider, windowKey)
	return rl.AllowWindow(ctx, key, pl.MaxRequests, pl.Window)
}
