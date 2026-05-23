package environment

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/adapters/ipquery"
	"github.com/ProacTrip/Backend/internal/modules/environment/adapters/openweather"
	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
	"github.com/ProacTrip/Backend/internal/modules/environment/features/get_destination_weather"
	"github.com/ProacTrip/Backend/internal/modules/environment/features/get_environment"
	"github.com/ProacTrip/Backend/internal/modules/environment/adapters/resolver"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

type Module struct {
	GetEnvironmentHandler *get_environment.Handler

	// EnvironmentResolver adapta el proveedor de geo-IP para el caso de uso
	// de registro del módulo auth (resuelve moneda/idioma/país/timezone desde IP).
	// Exportado para que bootstrap lo inyecte en la configuración del módulo auth.
	EnvironmentResolver *resolver.EnvironmentResolverAdapter

	// GetDestinationWeatherUC expone el caso de uso de clima de destino
	// para que el módulo search lo inyecte en el tool calling.
	GetDestinationWeatherUC *get_destination_weather.UseCase

	// wg contabiliza las goroutines fire-and-forget (escrituras asíncronas en caché).
	// Expuesto para que bootstrap llame a wg.Wait() durante el graceful shutdown.
	wg *sync.WaitGroup
}

type Config struct {
	OpenWeatherAPIKey   string
	OpenWeatherCacheTTL time.Duration
	OpenWeatherTimeout  time.Duration
	IpQueryBaseURL      string
	Cache               get_environment.Cache
	RateLimiter         *ratelimit.RateLimiter
}

// NewModule crea e inicializa el módulo environment.
// Lee configuración adicional de variables de entorno:
//
//   DEFAULT_COUNTRY_CODE — código de país por defecto (ej: "AR")
//   DEFAULT_CURRENCY — moneda por defecto (ej: "USD")
//   IPQUERY_TIMEOUT — timeout por intento HTTP en segundos (default: 5)
//   IPQUERY_MAX_RETRIES — reintentos máximos (default: 3)
func NewModule(cfg Config) *Module {
	defaultCountryCode := os.Getenv("DEFAULT_COUNTRY_CODE")
	if defaultCountryCode == "" {
		defaultCountryCode = "AR"
	}
	defaultCurrency := os.Getenv("DEFAULT_CURRENCY")
	if defaultCurrency == "" {
		defaultCurrency = "USD"
	}

	ipQueryTimeout := 5 * time.Second
	if v := os.Getenv("IPQUERY_TIMEOUT"); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
			ipQueryTimeout = time.Duration(seconds) * time.Second
		}
	}

	ipQueryMaxRetries := 3
	if v := os.Getenv("IPQUERY_MAX_RETRIES"); v != "" {
		if retries, err := strconv.Atoi(v); err == nil && retries >= 0 {
			ipQueryMaxRetries = retries
		}
	}

	var wg sync.WaitGroup

	ipQueryClient := ipquery.NewClient(cfg.IpQueryBaseURL, ipQueryTimeout, ipQueryMaxRetries)
	if cfg.OpenWeatherTimeout <= 0 {
		cfg.OpenWeatherTimeout = 10 * time.Second
	}
	openWeatherClient := openweather.NewClient(cfg.OpenWeatherAPIKey, cfg.OpenWeatherTimeout)

	if cfg.RateLimiter == nil {
		slog.Warn("Módulo environment: RateLimiter es nil — rate limiting deshabilitado para clima (usando noop)")
	}
	if cfg.Cache == nil {
		slog.Warn("Módulo environment: Cache es nil — caché será deshabilitado")
	}
	if cfg.OpenWeatherAPIKey == "" {
		slog.Warn("Módulo environment: OpenWeatherAPIKey está vacía — datos de clima no estarán disponibles")
	}

	getEnvironmentUC := get_environment.NewUseCase(get_environment.UseCaseDeps{
		LocationProvider:    ipQueryClient,
		WeatherProvider:     openWeatherClient,
		Cache:               cfg.Cache,
		RateLimiter:         cfg.RateLimiter,
		WeatherCacheTTL:     cfg.OpenWeatherCacheTTL,
		IpQueryCacheTTL:     24 * time.Hour,
		DefaultCountryCode:  defaultCountryCode,
		DefaultCurrency:     defaultCurrency,
		WG:                  &wg,
	})

	getEnvironmentHandler := get_environment.NewHandler(getEnvironmentUC)

	// Crear el caso de uso de clima de destino (forecast + histórico).
	// Usa el mismo cliente OpenWeather que get_environment y el mismo wg.
	// El caché usa el adaptador Dragonfly inyectado via Config.Cache.
	getDestWeatherUC := get_destination_weather.NewUseCase(get_destination_weather.UseCaseDeps{
		WeatherClient: openWeatherClient,
		Cache:         cfg.Cache,
		CacheTTL:      cfg.OpenWeatherCacheTTL,
		WG:            &wg,
	})

	// Crear el adaptador resolver para el wiring del registro en auth.
	// El adaptador usa el mismo cliente IP query (sin llamadas HTTP extra).
	resolverAdapter := resolver.NewEnvironmentResolverAdapter(ipQueryClient)

	// Registrar mapeos de errores de dominio → HTTP RFC 9457
	registerEnvironmentErrors()

	slog.Info("Módulo environment inicializado",
		"features", []string{"get_environment", "environment_resolver"},
		"ipquery_url", cfg.IpQueryBaseURL,
		"weather_cache_ttl", cfg.OpenWeatherCacheTTL,
		"ipquery_timeout", ipQueryTimeout,
		"ipquery_max_retries", ipQueryMaxRetries,
		"default_country_code", defaultCountryCode,
		"default_currency", defaultCurrency,
	)

	return &Module{
		GetEnvironmentHandler:   getEnvironmentHandler,
		EnvironmentResolver:     resolverAdapter,
		GetDestinationWeatherUC: getDestWeatherUC,
		wg:                      &wg,
	}
}

// Wait bloquea hasta que todas las goroutines fire-and-forget del módulo
// hayan terminado (ej: escrituras asíncronas en caché).
// Llamar durante graceful shutdown.
func (m *Module) Wait() {
	m.wg.Wait()
}

// registerEnvironmentErrors registra los mapeos de errores de dominio
// a respuestas HTTP RFC 9457 (Problem Details).
func registerEnvironmentErrors() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrInvalidIP):
			return serrors.ErrBadRequest("dirección IP inválida", err)
		case errors.Is(err, domain.ErrLocationProvider):
			return serrors.ErrBadGateway("proveedor de ubicación no disponible", err)
		case errors.Is(err, domain.ErrRateLimitExceeded):
			return serrors.ErrTooManyRequests("límite de peticiones excedido", err)
		case errors.Is(err, domain.ErrInternal):
			return serrors.ErrInternalError("error interno del servidor", err)
		}
		return nil
	})
}
