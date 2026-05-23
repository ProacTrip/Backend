// Caso de uso get_destination_weather — obtiene el clima para un destino en una fecha específica.
//
// Lógica de fecha:
//   - ≤7 días en el futuro → llama a forecast API (onecall con daily)
//   - >7 días en el futuro → llama a historical API (timemachine con dt = target - 1 año)
//
// Cache-aside con DragonflyDB v1.38: clave weather:dest:{blake3(lat:lng:date)}, TTL 10 minutos.
// Escritura asíncrona via wg.Go() — sigue el patrón de fetchWeather de get_environment.
//
// Degradación elegante: si el provider devuelve HTTP 429, retorna nil, nil (weather: null en SSE).
package get_destination_weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"lukechampine.com/blake3"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

// TTL de caché para weather de destino según recomendaciones DragonflyDB v1.38:
//   - External API (OpenWeather): 10min — balance entre frescura y costo de API
const defaultDestinationWeatherCacheTTL = 10 * time.Minute

// =============================================================================
// Interfaces
// =============================================================================

// WeatherForecaster es el puerto para obtener datos de clima de destino.
// Desacopla el usecase del proveedor concreto (OpenWeather).
type WeatherForecaster interface {
	GetForecastForDate(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error)
	GetHistoricalForDate(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error)
}

// DestinationWeatherCache abstrae las operaciones de caché necesarias para el usecase.
// Implementada por el adaptador Dragonfly (shared/cache) o mocks en tests.
type DestinationWeatherCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// noopCache es un caché que nunca encuentra entradas (útil cuando no hay caché configurado).
type noopCache struct{}

func (noopCache) Get(_ context.Context, _ string) (string, error) { return "", nil }
func (noopCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error { return nil }

// =============================================================================
// UseCase
// =============================================================================

// UseCase contiene la lógica de negocio para obtener el clima de un destino.
type UseCase struct {
	weatherClient WeatherForecaster
	cache         DestinationWeatherCache
	cacheTTL      time.Duration
	wg            *sync.WaitGroup
}

// UseCaseDeps agrupa las dependencias inyectables del caso de uso.
type UseCaseDeps struct {
	WeatherClient WeatherForecaster
	Cache         DestinationWeatherCache // nil = sin caché (usar noopCache)
	CacheTTL      time.Duration           // 0 = usar defaultDestinationWeatherCacheTTL
	WG            *sync.WaitGroup         // nil = crear uno nuevo
}

// NewUseCase crea una nueva instancia del caso de uso con sus dependencias.
func NewUseCase(deps UseCaseDeps) *UseCase {
	wg := deps.WG
	if wg == nil {
		wg = new(sync.WaitGroup)
	}
	cache := deps.Cache
	if cache == nil {
		cache = noopCache{}
	}
	ttl := deps.CacheTTL
	if ttl <= 0 {
		ttl = defaultDestinationWeatherCacheTTL
	}
	return &UseCase{
		weatherClient: deps.WeatherClient,
		cache:         cache,
		cacheTTL:      ttl,
		wg:            wg,
	}
}

// Wait bloquea hasta que todas las goroutines fire-and-forget hayan terminado.
func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

// =============================================================================
// Execute — orquestación principal
// =============================================================================

// Execute obtiene el clima para un destino en una fecha específica.
//
// Flujo:
//  1. Validar parámetros (lat, lng, date)
//  2. Verificar caché: weather:dest:{blake3(lat:lng:date)}
//  3. Determinar estrategia de fecha:
//     - ≤7 días → llamar forecast API (GetForecastForDate)
//     - >7 días → llamar historical API (GetHistoricalForDate)
//  4. Cachear resultado asíncronamente (fire-and-forget vía wg.Go)
//  5. Retornar WeatherData o nil,nil en caso de error 429 del provider
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Validar parámetros
	if err := cmd.Validate(); err != nil {
		slog.WarnContext(ctx, "get_destination_weather: validación fallida",
			slog.Float64("lat", cmd.Lat),
			slog.Float64("lng", cmd.Lng),
			slog.String("date", cmd.Date),
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	// 2. Verificar caché
	cacheKey := buildCacheKey(cmd.Lat, cmd.Lng, cmd.Date)
	if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var weather domain.WeatherData
		if err := json.Unmarshal([]byte(cached), &weather); err == nil {
			slog.DebugContext(ctx, "get_destination_weather: cache HIT",
				slog.String("key", cacheKey),
			)
			return &weather, nil
		}
		slog.WarnContext(ctx, "get_destination_weather: cache unmarshal falló, ignorando entrada",
			slog.String("key", cacheKey),
			slog.String("error", err.Error()),
		)
	}

	// 3. Determinar estrategia de fecha
	targetDate, err := time.Parse("2006-01-02", cmd.Date)
	if err != nil {
		return nil, fmt.Errorf("parse date %s: %w", cmd.Date, err)
	}
	now := time.Now().Truncate(24 * time.Hour)
	daysUntil := targetDate.Sub(now).Hours() / 24

	var weather *domain.WeatherData
	if daysUntil <= 7 {
		slog.DebugContext(ctx, "get_destination_weather: usando forecast (≤7d)",
			slog.Float64("days_until", daysUntil),
			slog.String("date", cmd.Date),
		)
		weather, err = uc.weatherClient.GetForecastForDate(ctx, cmd.Lat, cmd.Lng, cmd.Date)
	} else {
		slog.DebugContext(ctx, "get_destination_weather: usando historical (>7d)",
			slog.Float64("days_until", daysUntil),
			slog.String("date", cmd.Date),
		)
		weather, err = uc.weatherClient.GetHistoricalForDate(ctx, cmd.Lat, cmd.Lng, cmd.Date)
	}

	if err != nil {
		// Degradación elegante: HTTP 429 del provider → weather null, no error
		if errors.Is(err, domain.ErrWeatherProviderRateLimited) {
			slog.WarnContext(ctx, "get_destination_weather: provider rate limited, returning nil weather",
				slog.String("date", cmd.Date),
			)
			return nil, nil
		}
		// ErrNoWeatherData → weather null, no error
		if errors.Is(err, domain.ErrNoWeatherData) {
			slog.WarnContext(ctx, "get_destination_weather: no weather data available",
				slog.String("date", cmd.Date),
			)
			return nil, nil
		}
		slog.ErrorContext(ctx, "get_destination_weather: provider error",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("get destination weather: %w", err)
	}

	if weather == nil {
		slog.WarnContext(ctx, "get_destination_weather: provider returned nil (no API key?)")
		return nil, nil
	}

	// 4. Cachear resultado asíncronamente (fire-and-forget vía wg.Go)
	weatherCopy := *weather
	bgCtx := context.WithoutCancel(ctx)
	uc.wg.Go(func() {
		weatherBytes, marshalErr := json.Marshal(weatherCopy)
		if marshalErr != nil {
			slog.WarnContext(bgCtx, "get_destination_weather: fallo al serializar weather para caché",
				slog.String("error", marshalErr.Error()),
			)
			return
		}
		if cacheErr := uc.cache.Set(bgCtx, cacheKey, string(weatherBytes), uc.cacheTTL); cacheErr != nil {
			slog.WarnContext(bgCtx, "get_destination_weather: fallo al cachear (async)",
				slog.String("key", cacheKey),
				slog.String("error", cacheErr.Error()),
			)
		}
	})

	slog.DebugContext(ctx, "get_destination_weather: éxito",
		slog.Float64("temp", weather.Temp),
		slog.String("description", weather.Description),
	)

	return weather, nil
}

// =============================================================================
// Helpers
// =============================================================================

// buildCacheKey construye la clave de caché DragonflyDB.
// Formato: weather:dest:{blake3_hex(lat:lng:date)}
// blake3 garantiza una clave de longitud fija e determinística.
func buildCacheKey(lat, lng float64, date string) string {
	input := fmt.Sprintf("%.6f:%.6f:%s", lat, lng, date)
	hash := blake3.Sum256([]byte(input))
	return fmt.Sprintf("weather:dest:%x", hash[:])
}
