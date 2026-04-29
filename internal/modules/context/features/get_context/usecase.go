package get_context

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/context/domain"
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

func (uc *UseCase) Execute(ctx context.Context, ip, lang string) (*domain.ContextResponse, error) {
	slog.Error("get_context usecase: Execute called", "ip", ip, "lang", lang)

	slog.Error("get_context usecase: resolving location", "ip", ip)
	location, err := uc.resolveLocation(ctx, ip)
	if err != nil {
		slog.Error("get_context usecase: resolveLocation failed", "error", err, "ip", ip)
		return nil, fmt.Errorf("resolve ip: %w", err)
	}
	slog.Error("get_context usecase: location resolved",
		"country", location.Country,
		"country_code", location.CountryCode,
		"city", location.City,
		"lat", location.Latitude,
		"lon", location.Longitude,
	)

	location.Currency = domain.CurrencyForCountry(location.CountryCode)
	if lang == "en" {
		if countryLang := domain.LanguageForCountry(location.CountryCode); countryLang != "en" {
			lang = countryLang
		}
	}
	location.Language = lang
	slog.Error("get_context usecase: language resolved", "lang", lang, "currency", location.Currency)

	slog.Error("get_context usecase: fetching weather", "lat", location.Latitude, "lon", location.Longitude, "lang", lang)
	weather, err := uc.fetchWeather(ctx, location.Latitude, location.Longitude, lang)
	if err != nil {
		slog.Error("get_context usecase: fetchWeather failed", "error", err)
		return nil, fmt.Errorf("get weather: %w", err)
	}
	slog.Error("get_context usecase: weather fetched", "has_weather", weather != nil)

	return &domain.ContextResponse{
		Location: *location,
		Weather:  weather,
	}, nil
}

func (uc *UseCase) resolveLocation(ctx context.Context, ip string) (*domain.LocationData, error) {
	slog.Error("get_context usecase: resolveLocation", "ip", ip)
	if isLocalOrPrivate(ip) {
		slog.Error("get_context usecase: IP is local/private, using default location")
		return domain.DefaultLocation(), nil
	}

	if uc.locationProvider == nil {
		slog.Error("get_context usecase: FATAL — locationProvider is nil")
		return nil, fmt.Errorf("location provider not initialized")
	}

	slog.Error("get_context usecase: calling locationProvider.ResolveIP", "ip", ip)
	return uc.locationProvider.ResolveIP(ctx, ip)
}

func (uc *UseCase) fetchWeather(ctx context.Context, lat, lon float64, lang string) (*domain.WeatherData, error) {
	if uc.rateLimiter == nil {
		slog.Error("get_context usecase: FATAL — rateLimiter is nil, cannot enforce rate limits")
		return nil, fmt.Errorf("rate limiter not initialized")
	}

	slog.Error("get_context usecase: checking rate limit", "provider", "openweather")
	result, err := uc.rateLimiter.ProviderAllow(ctx, "openweather")
	if err != nil {
		slog.Error("get_context usecase: rate limit check failed", "error", err)
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	if !result.Allowed {
		slog.Error("get_context usecase: rate limit exceeded",
			"current", result.Current,
			"limit", result.Limit,
		)
		return nil, fmt.Errorf("openweather rate limit exceeded: %d/%d", result.Current, result.Limit)
	}
	slog.Error("get_context usecase: rate limit OK", "remaining", result.Remaining)

	roundedLat := math.Round(lat*100) / 100
	roundedLon := math.Round(lon*100) / 100
	cacheKey := fmt.Sprintf("weather:%.2f:%.2f:%s", roundedLat, roundedLon, lang)

	if uc.cache != nil {
		slog.Error("get_context usecase: checking cache", "key", cacheKey)
		if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
			var weather domain.WeatherData
			if err := json.Unmarshal([]byte(cached), &weather); err == nil {
				slog.Error("get_context usecase: cache HIT", "key", cacheKey)
				return &weather, nil
			}
			slog.Error("get_context usecase: cache unmarshal failed, ignoring", "error", err)
		}
	} else {
		slog.Error("get_context usecase: cache is nil, skipping cache lookup")
	}

	slog.Error("get_context usecase: calling weather provider", "lat", lat, "lon", lon, "lang", lang)
	weather, err := uc.weatherProvider.GetCurrentWeather(ctx, lat, lon, lang)
	if err != nil {
		slog.Error("get_context usecase: weather provider failed", "error", err)
		return nil, fmt.Errorf("get weather: %w", err)
	}
	if weather == nil {
		slog.Error("get_context usecase: weather provider returned nil (no API key?)")
		return nil, nil
	}

	slog.Error("get_context usecase: weather provider OK", "temp", weather.Temp, "description", weather.Description)

	if uc.cache != nil {
		weatherBytes, marshalErr := json.Marshal(weather)
		if marshalErr == nil {
			slog.Error("get_context usecase: caching weather", "key", cacheKey)
			_ = uc.cache.Set(ctx, cacheKey, string(weatherBytes), uc.cacheTTL)
		}
	}

	return weather, nil
}

func isLocalOrPrivate(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return true
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsUnspecified()
}
