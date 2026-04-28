package get_context

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	location, err := uc.locationProvider.ResolveIP(ctx, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve ip: %w", err)
	}

	result, err := uc.rateLimiter.ProviderAllow(ctx, "openweather")
	if err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	if !result.Allowed {
		return nil, fmt.Errorf("openweather rate limit exceeded: %d/%d", result.Current, result.Limit)
	}

	roundedLat := math.Round(location.Latitude*100) / 100
	roundedLon := math.Round(location.Longitude*100) / 100
	cacheKey := fmt.Sprintf("weather:%.2f:%.2f:%s", roundedLat, roundedLon, lang)

	if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var weather domain.WeatherData
		if err := json.Unmarshal([]byte(cached), &weather); err == nil {
			return &domain.ContextResponse{
				Location: *location,
				Weather:  weather,
			}, nil
		}
	}

	weather, err := uc.weatherProvider.GetCurrentWeather(ctx, location.Latitude, location.Longitude, lang)
	if err != nil {
		return nil, fmt.Errorf("get weather: %w", err)
	}

	weatherBytes, marshalErr := json.Marshal(weather)
	if marshalErr == nil {
		_ = uc.cache.Set(ctx, cacheKey, string(weatherBytes), uc.cacheTTL)
	}

	return &domain.ContextResponse{
		Location: *location,
		Weather:  *weather,
	}, nil
}
