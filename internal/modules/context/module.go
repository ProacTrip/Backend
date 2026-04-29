package context

import (
	"log/slog"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/context/adapters/ipquery"
	"github.com/ProacTrip/Backend/internal/modules/context/adapters/openweather"
	"github.com/ProacTrip/Backend/internal/modules/context/features/get_context"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

type Module struct {
	GetContextHandler *get_context.Handler
}

type Config struct {
	OpenWeatherAPIKey    string
	OpenWeatherCacheTTL  time.Duration
	IpQueryBaseURL       string
	Cache                get_context.Cache
	RateLimiter          *ratelimit.RateLimiter
}

func NewModule(cfg Config) *Module {
	ipQueryClient := ipquery.NewClient(cfg.IpQueryBaseURL)
	openWeatherClient := openweather.NewClient(cfg.OpenWeatherAPIKey)

	if cfg.RateLimiter == nil {
		slog.Warn("Context module: RateLimiter is nil — rate limiting will be disabled for weather")
	}
	if cfg.Cache == nil {
		slog.Warn("Context module: Cache is nil — cache will be disabled")
	}
	if cfg.OpenWeatherAPIKey == "" {
		slog.Warn("Context module: OpenWeatherAPIKey is empty — weather data will be unavailable")
	}

	getContextUC := get_context.NewUseCase(get_context.UseCaseDeps{
		LocationProvider: ipQueryClient,
		WeatherProvider:  openWeatherClient,
		Cache:            cfg.Cache,
		RateLimiter:      cfg.RateLimiter,
		CacheTTL:         cfg.OpenWeatherCacheTTL,
	})

	getContextHandler := get_context.NewHandler(getContextUC)

	slog.Info("Context module initialized",
		"features", []string{"get_context"},
		"ipquery_url", cfg.IpQueryBaseURL,
		"weather_cache_ttl", cfg.OpenWeatherCacheTTL,
	)

	return &Module{
		GetContextHandler: getContextHandler,
	}
}

func MustNewModule(cfg Config) *Module {
	return NewModule(cfg)
}
