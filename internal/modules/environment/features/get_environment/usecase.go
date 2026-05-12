package get_environment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
	sharedEnv "github.com/ProacTrip/Backend/internal/shared/environment"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// rateLimitChecker es la interfaz mínima que necesita el usecase del rate limiter.
// Desacopla el usecase de la implementación concreta de ratelimit.RateLimiter
// y permite inyectar un noop cuando el rate limiting no está configurado.
type rateLimitChecker interface {
	ProviderAllow(ctx context.Context, provider string) (ratelimit.RateLimitResult, error)
}

// noopRateLimiter es un rate limiter que siempre permite, usado cuando
// el rate limiting no está configurado. Evita el nil check en el hot path.
type noopRateLimiter struct{}

func (noopRateLimiter) ProviderAllow(_ context.Context, _ string) (ratelimit.RateLimitResult, error) {
	return ratelimit.RateLimitResult{Allowed: true, Remaining: -1}, nil
}

type UseCase struct {
	locationProvider   domain.LocationProvider
	weatherProvider    domain.WeatherProvider
	cache              Cache
	rateLimiter        rateLimitChecker
	cacheTTL           time.Duration
	defaultCountryCode string
	defaultCurrency    string
	wg                 *sync.WaitGroup
}

type UseCaseDeps struct {
	LocationProvider   domain.LocationProvider
	WeatherProvider    domain.WeatherProvider
	Cache              Cache
	RateLimiter        rateLimitChecker
	CacheTTL           time.Duration
	DefaultCountryCode string
	DefaultCurrency    string
	WG                 *sync.WaitGroup
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	wg := deps.WG
	if wg == nil {
		wg = new(sync.WaitGroup)
	}
	rateLimiter := deps.RateLimiter
	if rateLimiter == nil {
		rateLimiter = noopRateLimiter{}
	}
	return &UseCase{
		locationProvider:   deps.LocationProvider,
		weatherProvider:    deps.WeatherProvider,
		cache:              deps.Cache,
		rateLimiter:        rateLimiter,
		cacheTTL:           deps.CacheTTL,
		defaultCountryCode: deps.DefaultCountryCode,
		defaultCurrency:    deps.DefaultCurrency,
		wg:                 wg,
	}
}

// Wait bloquea hasta que todas las goroutines fire-and-forget hayan terminado.
func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

func (uc *UseCase) Execute(ctx context.Context, ip, lang string) (*domain.EnvironmentResponse, error) {
	slog.DebugContext(ctx, "get_environment usecase: Execute called", "ip", ip, "lang", lang)

	envCacheKey := sharedEnv.CacheKey(ip)
	if uc.cache != nil {
		slog.DebugContext(ctx, "get_environment usecase: checking env cache", "key", envCacheKey)
		if cached, err := uc.cache.Get(ctx, envCacheKey); err == nil && cached != "" {
			var cacheEntry domain.EnvironmentResponse
			if err := json.Unmarshal([]byte(cached), &cacheEntry); err == nil {
				slog.DebugContext(ctx, "get_environment usecase: env cache HIT", "key", envCacheKey)
				return &cacheEntry, nil
			}
			slog.WarnContext(ctx, "get_environment usecase: env cache unmarshal failed, ignoring", "error", err)
		}
	}

	slog.DebugContext(ctx, "get_environment usecase: resolving location", "ip", ip)
	location, err := uc.resolveLocation(ctx, ip)
	if err != nil {
		slog.ErrorContext(ctx, "get_environment usecase: resolveLocation failed", "error", err, "ip", ip)
		return nil, err
	}
	slog.DebugContext(ctx, "get_environment usecase: location resolved",
		"country", location.Country,
		"country_code", location.CountryCode,
		"city", location.City,
		"lat", location.Latitude,
		"lon", location.Longitude,
	)

	// Moneda desde CountryMetadata con fallback configurable.
	location.Currency = domain.CurrencyForCountry(location.CountryCode, uc.defaultCurrency)
	// Idioma desde Accept-Language, con fallback a CountryMetadata.
	if lang != "" {
		location.Language = lang
	} else {
		location.Language = domain.LanguageForCountry(location.CountryCode)
	}
	// Idioma de descripción del clima: desde Accept-Language, fallback a "en".
	weatherLang := lang
	if weatherLang == "" {
		weatherLang = "en"
	}
	slog.DebugContext(ctx, "get_environment usecase: metadata resolved",
		"lang", location.Language,
		"weather_lang", weatherLang,
		"currency", location.Currency,
	)

	slog.DebugContext(ctx, "get_environment usecase: fetching weather", "lat", location.Latitude, "lon", location.Longitude, "lang", weatherLang)
	weather, err := uc.fetchWeather(ctx, location.Latitude, location.Longitude, weatherLang)
	if err != nil {
		// Errores de rate limit deben propagarse (HTTP 429), no degradar con gracia.
		if isWeatherRateLimit(err) {
			return nil, fmt.Errorf("%w: %w", domain.ErrRateLimitExceeded, err)
		}
		slog.WarnContext(ctx, "weather fetch failed, continuing without weather", "error", err)
		weather = nil
	}
	slog.DebugContext(ctx, "get_environment usecase: weather fetched", "has_weather", weather != nil)

	response := &domain.EnvironmentResponse{
		Location: *location,
		Weather:  weather,
	}

	if uc.cache != nil {
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			envBytes, marshalErr := json.Marshal(response)
			if marshalErr != nil {
				slog.WarnContext(bgCtx, "fallo al serializar respuesta para caché", "error", marshalErr)
				return
			}
			slog.DebugContext(bgCtx, "get_environment usecase: caching env response (async)", "key", envCacheKey)
			if err := uc.cache.Set(bgCtx, envCacheKey, string(envBytes), uc.cacheTTL); err != nil {
				slog.WarnContext(bgCtx, "fallo al cachear environment (async)", "error", err, "key", envCacheKey)
			}
		})
	}

	return response, nil
}

// responseToCacheEntry convierte una EnvironmentResponse de dominio en una CacheEntry para el caché.
// Deprecado: la caché ahora almacena directamente domain.EnvironmentResponse.
// Se mantiene para compatibilidad con entradas existentes en caché.
func responseToCacheEntry(resp *domain.EnvironmentResponse) *sharedEnv.CacheEntry {
	entry := &sharedEnv.CacheEntry{
		Location: sharedEnv.LocationDTO{
			Country:     resp.Location.Country,
			CountryCode: resp.Location.CountryCode,
			City:        resp.Location.City,
			State:       resp.Location.State,
			Zipcode:     resp.Location.Zipcode,
			Timezone:    resp.Location.Timezone,
			Currency:    resp.Location.Currency,
			Language:    resp.Location.Language,
			Latitude:    resp.Location.Latitude,
			Longitude:   resp.Location.Longitude,
		},
	}
	if resp.Weather != nil {
		entry.Weather = &sharedEnv.WeatherDTO{
			Temp:        resp.Weather.Temp,
			FeelsLike:   resp.Weather.FeelsLike,
			Description: resp.Weather.Description,
			Icon:        resp.Weather.Icon,
			IconURL:     resp.Weather.IconURL,
			Humidity:    resp.Weather.Humidity,
			WindSpeed:   resp.Weather.WindSpeed,
		}
	}
	return entry
}

// cacheEntryToResponse convierte una CacheEntry del caché en una EnvironmentResponse de dominio.
func cacheEntryToResponse(entry *sharedEnv.CacheEntry) *domain.EnvironmentResponse {
	resp := &domain.EnvironmentResponse{
		Location: domain.LocationData{
			Country:     entry.Location.Country,
			CountryCode: entry.Location.CountryCode,
			City:        entry.Location.City,
			State:       entry.Location.State,
			Zipcode:     entry.Location.Zipcode,
			Timezone:    entry.Location.Timezone,
			Currency:    entry.Location.Currency,
			Language:    entry.Location.Language,
			Latitude:    entry.Location.Latitude,
			Longitude:   entry.Location.Longitude,
		},
	}
	if entry.Weather != nil {
		resp.Weather = &domain.WeatherData{
			Temp:        entry.Weather.Temp,
			FeelsLike:   entry.Weather.FeelsLike,
			Description: entry.Weather.Description,
			Icon:        entry.Weather.Icon,
			IconURL:     entry.Weather.IconURL,
			Humidity:    entry.Weather.Humidity,
			WindSpeed:   entry.Weather.WindSpeed,
		}
	}
	return resp
}

func (uc *UseCase) resolveLocation(ctx context.Context, ip string) (*domain.LocationData, error) {
	slog.DebugContext(ctx, "get_environment usecase: resolveLocation", "ip", ip)
	if isLocalOrPrivate(ip) {
		slog.InfoContext(ctx, "get_environment usecase: IP is local/private, attempting auto-detection via IP provider")
		if uc.locationProvider != nil {
			location, err := uc.locationProvider.ResolveIP(ctx, "")
			if err == nil {
				slog.DebugContext(ctx, "get_environment usecase: auto-detection succeeded",
					"country", location.Country,
					"country_code", location.CountryCode,
					"city", location.City,
				)
				return location, nil
			}
			slog.WarnContext(ctx, "get_environment usecase: auto-detection failed, falling back to default location", "error", err)
		}
		return domain.DefaultLocation(uc.defaultCountryCode), nil
	}

	if uc.locationProvider == nil {
		slog.ErrorContext(ctx, "get_environment usecase: FATAL — locationProvider is nil")
		return nil, fmt.Errorf("location provider not initialized")
	}

	slog.DebugContext(ctx, "get_environment usecase: calling locationProvider.ResolveIP", "ip", ip)
	location, err := uc.locationProvider.ResolveIP(ctx, ip)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrLocationProvider, err)
	}
	return location, nil
}

func (uc *UseCase) fetchWeather(ctx context.Context, lat, lon float64, lang string) (*domain.WeatherData, error) {
	slog.DebugContext(ctx, "get_environment usecase: checking rate limit", "provider", "openweather")
	result, err := uc.rateLimiter.ProviderAllow(ctx, "openweather")
	if err != nil {
		slog.ErrorContext(ctx, "get_environment usecase: rate limit check failed", "error", err)
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	if !result.Allowed {
		slog.WarnContext(ctx, "get_environment usecase: rate limit exceeded",
			"current", result.Current,
			"limit", result.Limit,
		)
		return nil, fmt.Errorf("openweather rate limit exceeded: %d/%d", result.Current, result.Limit)
	}
	slog.DebugContext(ctx, "get_environment usecase: rate limit OK", "remaining", result.Remaining)

	slog.DebugContext(ctx, "get_environment usecase: calling weather provider", "lat", lat, "lon", lon, "lang", lang)
	weather, err := uc.weatherProvider.GetCurrentWeather(ctx, lat, lon, lang)
	if err != nil {
		slog.ErrorContext(ctx, "get_environment usecase: weather provider failed", "error", err)
		if isWeatherRateLimit(err) {
			return nil, fmt.Errorf("%w: %w", domain.ErrRateLimitExceeded, err)
		}
		return nil, fmt.Errorf("get weather: %w", err)
	}
	if weather == nil {
		slog.WarnContext(ctx, "get_environment usecase: weather provider returned nil (no API key?)")
		return nil, nil
	}

	slog.DebugContext(ctx, "get_environment usecase: weather provider OK", "temp", weather.Temp, "description", weather.Description)

	return weather, nil
}

// isWeatherRateLimit detecta si un error del proveedor de clima indica
// rate limiting (HTTP 429) del servicio externo OpenWeather.
func isWeatherRateLimit(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "HTTP 429")
}

func isLocalOrPrivate(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return true
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsUnspecified()
}
