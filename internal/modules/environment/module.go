package environment

import (
	"log/slog"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/adapters/ipquery"
	"github.com/ProacTrip/Backend/internal/modules/environment/adapters/openweather"
	"github.com/ProacTrip/Backend/internal/modules/environment/features/get_environment"
	"github.com/ProacTrip/Backend/internal/modules/environment/features/shared"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

type Module struct {
	GetEnvironmentHandler *get_environment.Handler

	// EnvironmentResolver adapts the geo-IP location provider for auth module's
	// registration use case (resolves currency/language/country/timezone from IP).
	// Exported so bootstrap can wire it into the auth module config.
	EnvironmentResolver *shared.EnvironmentResolverAdapter
}

type Config struct {
	OpenWeatherAPIKey    string
	OpenWeatherCacheTTL  time.Duration
	IpQueryBaseURL       string
	Cache                get_environment.Cache
	RateLimiter          *ratelimit.RateLimiter
}

func NewModule(cfg Config) *Module {
	ipQueryClient := ipquery.NewClient(cfg.IpQueryBaseURL)
	openWeatherClient := openweather.NewClient(cfg.OpenWeatherAPIKey)

	if cfg.RateLimiter == nil {
		slog.Warn("Environment module: RateLimiter is nil — rate limiting will be disabled for weather")
	}
	if cfg.Cache == nil {
		slog.Warn("Environment module: Cache is nil — cache will be disabled")
	}
	if cfg.OpenWeatherAPIKey == "" {
		slog.Warn("Environment module: OpenWeatherAPIKey is empty — weather data will be unavailable")
	}

	getEnvironmentUC := get_environment.NewUseCase(get_environment.UseCaseDeps{
		LocationProvider: ipQueryClient,
		WeatherProvider:  openWeatherClient,
		Cache:            cfg.Cache,
		RateLimiter:      cfg.RateLimiter,
		CacheTTL:         cfg.OpenWeatherCacheTTL,
	})

	getEnvironmentHandler := get_environment.NewHandler(getEnvironmentUC)

	// Create the resolver adapter for auth registration wiring.
	// The adapter uses the same IP query client (no extra HTTP calls).
	resolverAdapter := shared.NewEnvironmentResolverAdapter(ipQueryClient)

	slog.Info("Environment module initialized",
		"features", []string{"get_environment", "environment_resolver"},
		"ipquery_url", cfg.IpQueryBaseURL,
		"weather_cache_ttl", cfg.OpenWeatherCacheTTL,
	)

	return &Module{
		GetEnvironmentHandler: getEnvironmentHandler,
		EnvironmentResolver:    resolverAdapter,
	}
}

func MustNewModule(cfg Config) *Module {
	return NewModule(cfg)
}
