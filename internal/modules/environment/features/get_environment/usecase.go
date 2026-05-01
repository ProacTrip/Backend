package get_environment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

type UseCase struct {
	locationProvider domain.LocationProvider
	weatherProvider  domain.WeatherProvider
	cache            Cache
	rateLimiter      *ratelimit.RateLimiter
	cacheTTL         time.Duration
}

type UseCaseDeps struct {
	LocationProvider domain.LocationProvider
	WeatherProvider  domain.WeatherProvider
	Cache            Cache
	RateLimiter      *ratelimit.RateLimiter
	CacheTTL         time.Duration
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		locationProvider: deps.LocationProvider,
		weatherProvider:  deps.WeatherProvider,
		cache:            deps.Cache,
		rateLimiter:      deps.RateLimiter,
		cacheTTL:         deps.CacheTTL,
	}
}

func (uc *UseCase) Execute(ctx context.Context, ip, lang string) (*domain.EnvironmentResponse, error) {
	slog.Debug("get_environment usecase: Execute called", "ip", ip, "lang", lang)

	envCacheKey := fmt.Sprintf("env:%s", ip)
	if uc.cache != nil {
		slog.Debug("get_environment usecase: checking env cache", "key", envCacheKey)
		if cached, err := uc.cache.Get(ctx, envCacheKey); err == nil && cached != "" {
			var response domain.EnvironmentResponse
			if err := json.Unmarshal([]byte(cached), &response); err == nil {
				slog.Debug("get_environment usecase: env cache HIT", "key", envCacheKey)
				return &response, nil
			}
			slog.Warn("get_environment usecase: env cache unmarshal failed, ignoring", "error", err)
		}
	}

	slog.Debug("get_environment usecase: resolving location", "ip", ip)
	location, err := uc.resolveLocation(ctx, ip)
	if err != nil {
		slog.Error("get_environment usecase: resolveLocation failed", "error", err, "ip", ip)
		return nil, fmt.Errorf("resolve ip: %w", err)
	}
	slog.Debug("get_environment usecase: location resolved",
		"country", location.Country,
		"country_code", location.CountryCode,
		"city", location.City,
		"lat", location.Latitude,
		"lon", location.Longitude,
	)

	// Currency comes from CountryMetadata (ipquery doesn't return it)
	location.Currency = domain.CurrencyForCountry(location.CountryCode)
	// location.Language: from Accept-Language, fallback to CountryMetadata
	if lang != "" {
		location.Language = lang
	} else {
		location.Language = domain.LanguageForCountry(location.CountryCode)
	}
	// Weather description language: from Accept-Language, fallback to "en"
	weatherLang := lang
	if weatherLang == "" {
		weatherLang = "en"
	}
	slog.Debug("get_environment usecase: metadata resolved",
		"lang", location.Language,
		"weather_lang", weatherLang,
		"currency", location.Currency,
	)

	slog.Debug("get_environment usecase: fetching weather", "lat", location.Latitude, "lon", location.Longitude, "lang", weatherLang)
	weather, err := uc.fetchWeather(ctx, location.Latitude, location.Longitude, weatherLang)
	if err != nil {
		slog.Warn("weather fetch failed, continuing without weather", "error", err)
		weather = nil
	}
	slog.Debug("get_environment usecase: weather fetched", "has_weather", weather != nil)

	response := &domain.EnvironmentResponse{
		Location: *location,
		Weather:  weather,
	}

	if uc.cache != nil {
		envBytes, marshalErr := json.Marshal(response)
		if marshalErr == nil {
			slog.Debug("get_environment usecase: caching env response", "key", envCacheKey)
			_ = uc.cache.Set(ctx, envCacheKey, string(envBytes), uc.cacheTTL)
		}
	}

	return response, nil
}

func (uc *UseCase) resolveLocation(ctx context.Context, ip string) (*domain.LocationData, error) {
	slog.Debug("get_environment usecase: resolveLocation", "ip", ip)
	if isLocalOrPrivate(ip) {
		slog.Info("get_environment usecase: IP is local/private, attempting auto-detection via IP provider")
		if uc.locationProvider != nil {
			location, err := uc.locationProvider.ResolveIP(ctx, "")
			if err == nil {
				slog.Debug("get_environment usecase: auto-detection succeeded",
					"country", location.Country,
					"country_code", location.CountryCode,
					"city", location.City,
				)
				return location, nil
			}
			slog.Warn("get_environment usecase: auto-detection failed, falling back to default location", "error", err)
		}
		return domain.DefaultLocation(), nil
	}

	if uc.locationProvider == nil {
		slog.Error("get_environment usecase: FATAL — locationProvider is nil")
		return nil, fmt.Errorf("location provider not initialized")
	}

	slog.Debug("get_environment usecase: calling locationProvider.ResolveIP", "ip", ip)
	return uc.locationProvider.ResolveIP(ctx, ip)
}

func (uc *UseCase) fetchWeather(ctx context.Context, lat, lon float64, lang string) (*domain.WeatherData, error) {
	if uc.rateLimiter == nil {
		slog.Warn("get_environment usecase: rate limiter not initialized — skipping rate limit check")
	} else {
		slog.Debug("get_environment usecase: checking rate limit", "provider", "openweather")
		result, err := uc.rateLimiter.ProviderAllow(ctx, "openweather")
		if err != nil {
			slog.Error("get_environment usecase: rate limit check failed", "error", err)
			return nil, fmt.Errorf("rate limit: %w", err)
		}
		if !result.Allowed {
			slog.Warn("get_environment usecase: rate limit exceeded",
				"current", result.Current,
				"limit", result.Limit,
			)
			return nil, fmt.Errorf("openweather rate limit exceeded: %d/%d", result.Current, result.Limit)
		}
		slog.Debug("get_environment usecase: rate limit OK", "remaining", result.Remaining)
	}

	slog.Debug("get_environment usecase: calling weather provider", "lat", lat, "lon", lon, "lang", lang)
	weather, err := uc.weatherProvider.GetCurrentWeather(ctx, lat, lon, lang)
	if err != nil {
		slog.Error("get_environment usecase: weather provider failed", "error", err)
		return nil, fmt.Errorf("get weather: %w", err)
	}
	if weather == nil {
		slog.Warn("get_environment usecase: weather provider returned nil (no API key?)")
		return nil, nil
	}

	slog.Debug("get_environment usecase: weather provider OK", "temp", weather.Temp, "description", weather.Description)

	return weather, nil
}

func isLocalOrPrivate(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return true
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsUnspecified()
}
