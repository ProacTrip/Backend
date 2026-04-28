// Paquete de rate limiting: configura límites por tipo de usuario y proveedor.
// Proporciona默认值 y carga desde variables de entorno.
package ratelimit

import (
	"fmt"
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

func DefaultConfig() *RateLimitConfig {
	return &RateLimitConfig{
		GlobalPerMinute:        100,
		AuthenticatedPerMinute: 10,
		AnonymousPerMinute:     5,
		Providers: map[string]ProviderLimit{
			"resend":      {MaxRequests: 100, Window: 24 * time.Hour},
			"serpapi":     {MaxRequests: 50, Window: 1 * time.Hour},
			"openweather": {MaxRequests: 1000, Window: 24 * time.Hour},
		},
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

	for _, name := range []string{"resend", "serpapi", "openweather"} {
		prefix := fmt.Sprintf("RATELIMIT_PROVIDER_%s", strings.ToUpper(name))
		maxKey := prefix + "_MAX"
		windowKey := prefix + "_WINDOW_SEC"

		max := getEnvInt(maxKey)
		windowSec := getEnvInt(windowKey)

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
