package environment

import (
	"log/slog"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/adapters/ipquery"
	"github.com/ProacTrip/Backend/internal/modules/environment/adapters/openweather"
	"github.com/ProacTrip/Backend/internal/modules/environment/features/get_environment"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

type Module struct {
	GetEnvironmentHandler *get_environment.Handler
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

	slog.Info("Environment module initialized",
		"features", []string{"get_environment"},
		"ipquery_url", cfg.IpQueryBaseURL,
		"weather_cache_ttl", cfg.OpenWeatherCacheTTL,
	)

	return &Module{
		GetEnvironmentHandler: getEnvironmentHandler,
	}
}

func MustNewModule(cfg Config) *Module {
	return NewModule(cfg)
}
