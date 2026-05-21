package get_environment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
	sharedEnv "github.com/ProacTrip/Backend/internal/shared/environment"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

// TTLs de caché según recomendaciones de DragonflyDB v1.38:
//   - GeoIP (ipquery): 24h — estable por IP, llamada gratuita
//   - External API (OpenWeather): 30min — ahorra costos, datos del provider refrescan cada ~10min
//     El frontend controla la frecuencia de refresco, el backend cachea agresivamente.
const (
	defaultIpQueryCacheTTL  = 24 * time.Hour
	// weatherLatLonPrecision define la precisión de redondeo de lat/lon para
	// la clave de caché de clima. 2 decimales ≈ 1.1km — suficiente para que
	// todos los usuarios de una misma ciudad compartan la misma entrada.
	weatherLatLonPrecision = 2
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
	locationProvider    domain.LocationProvider
	weatherProvider     domain.WeatherProvider
	cache               Cache
	rateLimiter         rateLimitChecker
	weatherCacheTTL     time.Duration
	ipQueryCacheTTL     time.Duration
	defaultCountryCode  string
	defaultCurrency     string
	wg                  *sync.WaitGroup
}

type UseCaseDeps struct {
	LocationProvider    domain.LocationProvider
	WeatherProvider     domain.WeatherProvider
	Cache               Cache
	RateLimiter         rateLimitChecker
	WeatherCacheTTL     time.Duration
	IpQueryCacheTTL     time.Duration
	DefaultCountryCode  string
	DefaultCurrency     string
	WG                  *sync.WaitGroup
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
	ipQueryTTL := deps.IpQueryCacheTTL
	if ipQueryTTL <= 0 {
		ipQueryTTL = defaultIpQueryCacheTTL
	}
	weatherTTL := deps.WeatherCacheTTL
	if weatherTTL <= 0 {
		weatherTTL = 30 * time.Minute
	}
	return &UseCase{
		locationProvider:   deps.LocationProvider,
		weatherProvider:    deps.WeatherProvider,
		cache:              deps.Cache,
		rateLimiter:        rateLimiter,
		weatherCacheTTL:    weatherTTL,
		ipQueryCacheTTL:    ipQueryTTL,
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

	// 1. Resolver ubicación (con caché de 24h en ipquery:{ip})
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

	// 2. Enriquecer con metadatos (moneda, idioma)
	// Moneda desde CountryMetadata con fallback configurable.
	location.Currency = domain.CurrencyForCountry(location.CountryCode, uc.defaultCurrency)
	// Idioma desde Accept-Language, con fallback a CountryMetadata.
	if lang != "" {
		location.Language = lang
	} else {
		location.Language = domain.LanguageForCountry(location.CountryCode)
	}
	// Idioma de descripción del clima: desde Accept-Language, fallback a "es".
	weatherLang := lang
	if weatherLang == "" {
		weatherLang = "es"
	}
	// Unidad de temperatura según el país: la mayoría usa Celsius, EE.UU. y Liberia usan Fahrenheit.
	tempUnit := domain.TempUnitForCountry(location.CountryCode)
	slog.DebugContext(ctx, "get_environment usecase: metadata resolved",
		"lang", location.Language,
		"weather_lang", weatherLang,
		"currency", location.Currency,
		"temp_unit", tempUnit,
	)

	// Escribir CountryInfo enriquecido en la clave compartida env:{ip} para el módulo search.
	// Se hace asíncrono después del enriquecimiento para que currency y language estén poblados.
	if uc.cache != nil {
		locForShared := *location
		sharedIP := ip
		sharedTTL := uc.ipQueryCacheTTL
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			countryKey := sharedEnv.CacheKey(sharedIP)
			countryEntry := sharedEnv.CacheEntry{
				Location: sharedEnv.LocationDTO{
					Country:     locForShared.Country,
					CountryCode: locForShared.CountryCode,
					City:        locForShared.City,
					State:       locForShared.State,
					Zipcode:     locForShared.Zipcode,
					Timezone:    locForShared.Timezone,
					Currency:    locForShared.Currency,
					Language:    locForShared.Language,
					Latitude:    locForShared.Latitude,
					Longitude:   locForShared.Longitude,
				},
			}
			countryBytes, marshalErr := json.Marshal(countryEntry)
			if marshalErr != nil {
				slog.WarnContext(bgCtx, "fallo al serializar country info para caché compartida", "error", marshalErr)
				return
			}
			if cacheErr := uc.cache.Set(bgCtx, countryKey, string(countryBytes), sharedTTL); cacheErr != nil {
				slog.WarnContext(bgCtx, "fallo al cachear country info (async)", "error", cacheErr, "key", countryKey)
			}
		})
	}

	// 3. Obtener clima (con caché por lat/lon redondeado, 30min TTL)
	slog.DebugContext(ctx, "get_environment usecase: fetching weather", "lat", location.Latitude, "lon", location.Longitude, "lang", weatherLang, "units", tempUnit)
	weather, err := uc.fetchWeather(ctx, location.Latitude, location.Longitude, weatherLang, tempUnit)
	if err != nil {
		// Errores de rate limit deben propagarse (HTTP 429), no degradar con gracia.
		if errors.Is(err, domain.ErrRateLimitExceeded) {
			return nil, err
		}
		slog.WarnContext(ctx, "weather fetch failed, continuing without weather", "error", err)
		weather = nil
	}
	slog.DebugContext(ctx, "get_environment usecase: weather fetched", "has_weather", weather != nil)

	return &domain.EnvironmentResponse{
		Location: *location,
		Weather:  weather,
	}, nil
}

// resolveLocation resuelve la ubicación geográfica para una IP.
// Cachea el resultado en DragonflyDB con clave ipquery:{ip} y TTL de 24h.
// Para IPs privadas/locales, intenta auto-detección vía el provider (IP vacía).
func (uc *UseCase) resolveLocation(ctx context.Context, ip string) (*domain.LocationData, error) {
	slog.DebugContext(ctx, "get_environment usecase: resolveLocation", "ip", ip)

	ipQueryKey := "ipquery:" + ip

	// Cache-aside: leer de caché primero
	if uc.cache != nil {
		if cached, err := uc.cache.Get(ctx, ipQueryKey); err == nil && cached != "" {
			var loc domain.LocationData
			if err := json.Unmarshal([]byte(cached), &loc); err == nil {
				slog.DebugContext(ctx, "get_environment usecase: ipquery cache HIT", "key", ipQueryKey)
				return &loc, nil
			}
			slog.WarnContext(ctx, "get_environment usecase: ipquery cache unmarshal failed, ignoring", "error", err)
		}
	}

	// Cache miss — resolver ubicación
	var location *domain.LocationData
	var err error

	if domain.IsPrivateOrLocalIP(ip) {
		slog.InfoContext(ctx, "get_environment usecase: IP is local/private, attempting auto-detection via IP provider")
		if uc.locationProvider != nil {
			location, err = uc.locationProvider.ResolveIP(ctx, "")
			if err == nil {
				slog.DebugContext(ctx, "get_environment usecase: auto-detection succeeded",
					"country", location.Country,
					"country_code", location.CountryCode,
					"city", location.City,
				)
			} else {
				slog.WarnContext(ctx, "get_environment usecase: auto-detection failed, falling back to default location", "error", err)
				return domain.DefaultLocation(uc.defaultCountryCode), nil
			}
		} else {
			return domain.DefaultLocation(uc.defaultCountryCode), nil
		}
	} else {
		if uc.locationProvider == nil {
			slog.ErrorContext(ctx, "get_environment usecase: FATAL — locationProvider is nil")
			return nil, fmt.Errorf("location provider not initialized")
		}
		slog.DebugContext(ctx, "get_environment usecase: calling locationProvider.ResolveIP", "ip", ip)
		location, err = uc.locationProvider.ResolveIP(ctx, ip)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", domain.ErrLocationProvider, err)
		}
	}

	// Cachear ubicación asíncronamente (fire-and-forget)
	if uc.cache != nil && location != nil {
		locCopy := *location
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			locBytes, marshalErr := json.Marshal(locCopy)
			if marshalErr != nil {
				slog.WarnContext(bgCtx, "fallo al serializar location para caché", "error", marshalErr)
				return
			}
			slog.DebugContext(bgCtx, "get_environment usecase: caching ipquery result (async)", "key", ipQueryKey, "ttl", uc.ipQueryCacheTTL)
			if cacheErr := uc.cache.Set(bgCtx, ipQueryKey, string(locBytes), uc.ipQueryCacheTTL); cacheErr != nil {
				slog.WarnContext(bgCtx, "fallo al cachear ipquery (async)", "error", cacheErr, "key", ipQueryKey)
			}
		})
	}

	return location, nil
}

// fetchWeather obtiene el clima para unas coordenadas dadas, con caché por lat/lon
// redondeado para maximizar cache hits entre usuarios de la misma zona.
//
// Estrategia de caché (DragonflyDB v1.38):
//   - Clave: weather:{lat_2d}:{lon_2d}:{lang} — 2 decimales ≈ 1.1km de precisión
//   - TTL: configurable (default 30min) — el provider refresca cada ~10min,
//     pero el frontend controla la frecuencia de peticiones
//   - Rate limiting interno vía RateLimiter.ProviderAllow antes de llamar al provider
func (uc *UseCase) fetchWeather(ctx context.Context, lat, lon float64, lang, units string) (*domain.WeatherData, error) {
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

	// Redondear coordenadas para la clave de caché (compartir clima entre IPs cercanas)
	roundedLat := roundTo(lat, weatherLatLonPrecision)
	roundedLon := roundTo(lon, weatherLatLonPrecision)
	weatherKey := fmt.Sprintf("weather:%.*f:%.*f:%s:%s", weatherLatLonPrecision, roundedLat, weatherLatLonPrecision, roundedLon, lang, units)

	// Cache-aside: leer de caché primero
	if uc.cache != nil {
		if cached, err := uc.cache.Get(ctx, weatherKey); err == nil && cached != "" {
			var weather domain.WeatherData
			if err := json.Unmarshal([]byte(cached), &weather); err == nil {
				slog.DebugContext(ctx, "get_environment usecase: weather cache HIT", "key", weatherKey)
				return &weather, nil
			}
			slog.WarnContext(ctx, "get_environment usecase: weather cache unmarshal failed, ignoring", "error", err)
		}
	}

	// Cache miss — llamar al provider externo (cuesta dinero)
	slog.DebugContext(ctx, "get_environment usecase: calling weather provider", "lat", lat, "lon", lon, "lang", lang, "units", units)
	weather, err := uc.weatherProvider.GetCurrentWeather(ctx, lat, lon, lang, units)
	if err != nil {
		slog.ErrorContext(ctx, "get_environment usecase: weather provider failed", "error", err)
		// Detectar rate limiting del proveedor externo via centinela (no string matching).
		if errors.Is(err, domain.ErrWeatherProviderRateLimited) {
			return nil, fmt.Errorf("%w: %w", domain.ErrRateLimitExceeded, err)
		}
		return nil, fmt.Errorf("get weather: %w", err)
	}
	if weather == nil {
		slog.WarnContext(ctx, "get_environment usecase: weather provider returned nil (no API key?)")
		return nil, nil
	}

	slog.DebugContext(ctx, "get_environment usecase: weather provider OK", "temp", weather.Temp, "description", weather.Description)

	// Cachear clima asíncronamente (fire-and-forget)
	if uc.cache != nil {
		weatherCopy := *weather
		bgCtx := context.WithoutCancel(ctx)
		uc.wg.Go(func() {
			weatherBytes, marshalErr := json.Marshal(weatherCopy)
			if marshalErr != nil {
				slog.WarnContext(bgCtx, "fallo al serializar weather para caché", "error", marshalErr)
				return
			}
			slog.DebugContext(bgCtx, "get_environment usecase: caching weather (async)", "key", weatherKey, "ttl", uc.weatherCacheTTL)
			if cacheErr := uc.cache.Set(bgCtx, weatherKey, string(weatherBytes), uc.weatherCacheTTL); cacheErr != nil {
				slog.WarnContext(bgCtx, "fallo al cachear weather (async)", "error", cacheErr, "key", weatherKey)
			}
		})
	}

	return weather, nil
}

// roundTo redondea un float64 a n decimales.
func roundTo(val float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(val*pow) / pow
}


