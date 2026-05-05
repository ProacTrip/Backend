// Paquete de rate limiting: configura límites por tipo de usuario y proveedor.
// Proporciona valores por defecto y carga desde variables de entorno.
package ratelimit

import (
	"maps"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProviderLimit struct {
	MaxRequests int
	Window      time.Duration
}

type RateLimitConfig struct {
	GlobalPerMinute        int
	AuthenticatedPerMinute int
	AnonymousPerMinute     int
	Providers              map[string]ProviderLimit
}

// DefaultProviderLimits defines the default provider rate limits.
// Providers: resend (email), serpapi (flights/hotels), openweather (weather).
var DefaultProviderLimits = map[string]ProviderLimit{
	"resend":      {MaxRequests: 100, Window: 24 * time.Hour},
	"serpapi":     {MaxRequests: 50, Window: 1 * time.Hour},
	"openweather": {MaxRequests: 1000, Window: 24 * time.Hour},
	"ai":          {MaxRequests: 10, Window: 1 * time.Hour},
}

func DefaultConfig() *RateLimitConfig {
	return &RateLimitConfig{
		GlobalPerMinute:        100,
		AuthenticatedPerMinute: 10,
		AnonymousPerMinute:     5,
		Providers:              maps.Clone(DefaultProviderLimits),
	}
}

func LoadRateLimitConfig() *RateLimitConfig {
	cfg := DefaultConfig()

	if v := getEnvInt("RATELIMIT_GLOBAL_PER_MINUTE"); v > 0 {
		cfg.GlobalPerMinute = v
	}
	if v := getEnvInt("RATELIMIT_AUTH_PER_MINUTE"); v > 0 {
		cfg.AuthenticatedPerMinute = v
	}
	if v := getEnvInt("RATELIMIT_ANON_PER_MINUTE"); v > 0 {
		cfg.AnonymousPerMinute = v
	}

	// Scan env vars once — overrides defaults AND discovers new providers.
	const envPrefix = "RATELIMIT_PROVIDER_"
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		if !strings.HasPrefix(key, envPrefix) {
			continue
		}
		// Extract provider name: RATELIMIT_PROVIDER_SERPAPI_MAX → serpapi
		suffix := key[len(envPrefix):]
		parts := strings.SplitN(suffix, "_", 2)
		if len(parts) != 2 || parts[1] != "MAX" {
			continue
		}
		name := strings.ToLower(parts[0])
		prefix := envPrefix + strings.ToUpper(name)
		max := getEnvInt(prefix + "_MAX")
		windowSec := getEnvInt(prefix + "_WINDOW_SEC")
		if max > 0 && windowSec > 0 {
			cfg.Providers[name] = ProviderLimit{
				MaxRequests: max,
				Window:      time.Duration(windowSec) * time.Second,
			}
		}
	}

	return cfg
}

// =============================================================================
// Helper para leer variables de entorno como int
// =============================================================================

func getEnvInt(key string) int {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}
