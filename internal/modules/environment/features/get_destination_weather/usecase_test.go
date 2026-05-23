// Tests unitarios para el caso de uso get_destination_weather.
// Usa synctest para pruebas determinísticas de concurrencia con wg.Go().
package get_destination_weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockWeatherForecaster struct {
	forecastFn   func(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error)
	historicalFn func(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error)
}

func (m *mockWeatherForecaster) GetForecastForDate(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
	if m.forecastFn != nil {
		return m.forecastFn(ctx, lat, lng, date)
	}
	return nil, nil
}

func (m *mockWeatherForecaster) GetHistoricalForDate(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
	if m.historicalFn != nil {
		return m.historicalFn(ctx, lat, lng, date)
	}
	return nil, nil
}

type mockDestCache struct {
	getFn    func(ctx context.Context, key string) (string, error)
	setFn    func(ctx context.Context, key string, value any, ttl time.Duration) error
	setCalls []setCall
}

type setCall struct {
	key   string
	value string
	ttl   time.Duration
}

func (m *mockDestCache) Get(ctx context.Context, key string) (string, error) {
	if m.getFn != nil {
		return m.getFn(ctx, key)
	}
	return "", nil
}

func (m *mockDestCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	m.setCalls = append(m.setCalls, setCall{key: key, value: fmt.Sprintf("%v", value), ttl: ttl})
	if m.setFn != nil {
		return m.setFn(ctx, key, value, ttl)
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

func makeWeatherData() *domain.WeatherData {
	return &domain.WeatherData{
		Temp:        28.5,
		FeelsLike:   30.2,
		Description: "cielo claro",
		Icon:        "01d",
		IconURL:     "https://openweathermap.org/img/wn/01d@4x.png",
		Humidity:    55,
		WindSpeed:   3.6,
	}
}

func futureDate(daysFromNow int) string {
	return time.Now().AddDate(0, 0, daysFromNow).Format("2006-01-02")
}

func pastDate() string {
	return time.Now().AddDate(0, 0, -1).Format("2006-01-02")
}

// =============================================================================
// Tests — Execute
// =============================================================================

func TestExecute_ForecastRoute_Within7Days(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		mockClient := &mockWeatherForecaster{
			forecastFn: func(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
				return makeWeatherData(), nil
			},
		}
		cache := &mockDestCache{}

		uc := NewUseCase(UseCaseDeps{
			WeatherClient: mockClient,
			Cache:         cache,
			WG:            new(sync.WaitGroup),
		})

		cmd := Command{
			Lat:  41.38,
			Lng:  2.17,
			Date: futureDate(3), // 3 days from now
		}

		weather, err := uc.Execute(ctx, cmd)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if weather == nil {
			t.Fatal("expected weather data, got nil")
		}
		if weather.Temp != 28.5 {
			t.Errorf("Temp = %.1f, want 28.5", weather.Temp)
		}
		if weather.Description != "cielo claro" {
			t.Errorf("Description = %q, want 'cielo claro'", weather.Description)
		}

		// Cache should have been written asynchronously
		synctest.Wait()
		if len(cache.setCalls) != 1 {
			t.Errorf("expected 1 cache write, got %d", len(cache.setCalls))
		}
	})
}

func TestExecute_HistoricalRoute_Over7Days(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		mockClient := &mockWeatherForecaster{
			historicalFn: func(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
				return makeWeatherData(), nil
			},
		}
		cache := &mockDestCache{}

		uc := NewUseCase(UseCaseDeps{
			WeatherClient: mockClient,
			Cache:         cache,
			WG:            new(sync.WaitGroup),
		})

		cmd := Command{
			Lat:  -34.60,
			Lng:  -58.38,
			Date: futureDate(30), // 30 days from now → historical
		}

		weather, err := uc.Execute(ctx, cmd)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if weather == nil {
			t.Fatal("expected weather data, got nil")
		}
		if weather.Temp != 28.5 {
			t.Errorf("Temp = %.1f, want 28.5", weather.Temp)
		}
	})
}

func TestExecute_InvalidLatitude(t *testing.T) {
	t.Parallel()

	uc := NewUseCase(UseCaseDeps{
		WeatherClient: &mockWeatherForecaster{},
		Cache:         &mockDestCache{},
	})

	cmd := Command{
		Lat:  91.0,
		Lng:  2.17,
		Date: futureDate(2),
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("expected error for invalid latitude, got nil")
	}
}

func TestExecute_PastDate(t *testing.T) {
	t.Parallel()

	uc := NewUseCase(UseCaseDeps{
		WeatherClient: &mockWeatherForecaster{},
		Cache:         &mockDestCache{},
	})

	cmd := Command{
		Lat:  41.38,
		Lng:  2.17,
		Date: pastDate(),
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("expected error for past date, got nil")
	}
}

func TestExecute_EmptyDate(t *testing.T) {
	t.Parallel()

	uc := NewUseCase(UseCaseDeps{
		WeatherClient: &mockWeatherForecaster{},
		Cache:         &mockDestCache{},
	})

	cmd := Command{
		Lat:  41.38,
		Lng:  2.17,
		Date: "",
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("expected error for empty date, got nil")
	}
}

func TestExecute_429GracefulFallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		mockClient := &mockWeatherForecaster{
			forecastFn: func(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
				return nil, fmt.Errorf("%w: rate limit", domain.ErrWeatherProviderRateLimited)
			},
		}
		cache := &mockDestCache{}

		uc := NewUseCase(UseCaseDeps{
			WeatherClient: mockClient,
			Cache:         cache,
			WG:            new(sync.WaitGroup),
		})

		cmd := Command{
			Lat:  41.38,
			Lng:  2.17,
			Date: futureDate(2),
		}

		weather, err := uc.Execute(ctx, cmd)
		if err != nil {
			t.Fatalf("expected nil error (graceful fallback), got: %v", err)
		}
		if weather != nil {
			t.Errorf("expected nil weather on 429, got: %+v", weather)
		}
	})
}

func TestExecute_NoWeatherDataGracefulFallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		mockClient := &mockWeatherForecaster{
			forecastFn: func(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
				return nil, fmt.Errorf("%w: no data", domain.ErrNoWeatherData)
			},
		}
		cache := &mockDestCache{}

		uc := NewUseCase(UseCaseDeps{
			WeatherClient: mockClient,
			Cache:         cache,
			WG:            new(sync.WaitGroup),
		})

		cmd := Command{
			Lat:  41.38,
			Lng:  2.17,
			Date: futureDate(2),
		}

		weather, err := uc.Execute(ctx, cmd)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if weather != nil {
			t.Errorf("expected nil weather on ErrNoWeatherData, got: %+v", weather)
		}
	})
}

func TestExecute_CacheHit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		weatherData := makeWeatherData()
		cachedJSON, _ := json.Marshal(weatherData)

		cache := &mockDestCache{
			getFn: func(ctx context.Context, key string) (string, error) {
				return string(cachedJSON), nil
			},
		}

		forecastCalled := false
		mockClient := &mockWeatherForecaster{
			forecastFn: func(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
				forecastCalled = true
				return weatherData, nil
			},
		}

		uc := NewUseCase(UseCaseDeps{
			WeatherClient: mockClient,
			Cache:         cache,
			WG:            new(sync.WaitGroup),
		})

		cmd := Command{
			Lat:  41.38,
			Lng:  2.17,
			Date: futureDate(2),
		}

		weather, err := uc.Execute(ctx, cmd)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if weather == nil {
			t.Fatal("expected weather data from cache, got nil")
		}
		if forecastCalled {
			t.Error("cache hit should NOT call the forecaster")
		}
	})
}

func TestExecute_InvalidLongitude(t *testing.T) {
	t.Parallel()

	uc := NewUseCase(UseCaseDeps{
		WeatherClient: &mockWeatherForecaster{},
		Cache:         &mockDestCache{},
	})

	cmd := Command{
		Lat:  41.38,
		Lng:  181.0,
		Date: futureDate(2),
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("expected error for invalid longitude, got nil")
	}
}

func TestBuildCacheKey_Deterministic(t *testing.T) {
	t.Parallel()

	k1 := buildCacheKey(41.38, 2.17, "2026-08-15")
	k2 := buildCacheKey(41.38, 2.17, "2026-08-15")
	k3 := buildCacheKey(41.39, 2.17, "2026-08-15")

	if k1 != k2 {
		t.Errorf("same params should produce same key: k1=%q, k2=%q", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("different params should produce different keys: k1=%q, k3=%q", k1, k3)
	}
	if k1[:13] != "weather:dest:" {
		t.Errorf("cache key should start with 'weather:dest:', got: %q", k1[:13])
	}
}

func TestNoopCache_AlwaysMiss(t *testing.T) {
	t.Parallel()

	nc := noopCache{}
	val, err := nc.Get(t.Context(), "any-key")
	if err != nil {
		t.Fatalf("noopCache.Get should not error: %v", err)
	}
	if val != "" {
		t.Errorf("noopCache.Get should return empty string, got: %q", val)
	}

	err = nc.Set(t.Context(), "any-key", "value", time.Minute)
	if err != nil {
		t.Fatalf("noopCache.Set should not error: %v", err)
	}
}

// =============================================================================
// Mock verification — errors.Is checks
// =============================================================================

func TestErrWeatherProviderRateLimited_IsDetected(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("%w: rate limit", domain.ErrWeatherProviderRateLimited)
	if !errors.Is(err, domain.ErrWeatherProviderRateLimited) {
		t.Error("errors.Is should detect ErrWeatherProviderRateLimited in wrapped error")
	}
}

func TestErrNoWeatherData_IsDetected(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("%w: no data", domain.ErrNoWeatherData)
	if !errors.Is(err, domain.ErrNoWeatherData) {
		t.Error("errors.Is should detect ErrNoWeatherData in wrapped error")
	}
}
