// Módulo de búsqueda de vuelos — punto de entrada y wiring del módulo search.
// Expone los handlers HTTP y las dependencias necesarias.
package search

import (
	"errors"
	"log/slog"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/search/adapters/serpapi"
	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/flight_details"
	"github.com/ProacTrip/Backend/internal/modules/search/features/hotel_details"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

// =============================================================================
// Estructura del Módulo
// =============================================================================

// Module expone todos los handlers y dependencias del módulo Search.
type Module struct {
	SearchFlightsHandler *search_flights.Handler
	FlightDetailsHandler *flight_details.Handler
	SearchHotelsHandler  *search_hotels.Handler
	HotelDetailsHandler  *hotel_details.Handler
	Repository           domain.SearchHistoryRepository

	searchUC  *search_flights.UseCase
	detailsUC *flight_details.UseCase
	hotelsUC  *search_hotels.UseCase
	hdetailsUC *hotel_details.UseCase
}

// Wait blocks until all fire-and-forget goroutines have completed.
// Call during graceful shutdown to avoid losing in-flight cache writes or history saves.
func (m *Module) Wait() {
	m.searchUC.Wait()
	m.detailsUC.Wait()
	m.hotelsUC.Wait()
	m.hdetailsUC.Wait()
}

// =============================================================================
// Configuración del Módulo
// =============================================================================

// Config contiene la configuración del módulo Search.
type Config struct {
	SerpAPIKey     string
	SerpAPITimeout time.Duration

	// Provider — la implementación de FlightProvider (normalmente serpapi.Adapter)
	// Si es nil, se crea desde SerpAPIKey/SerpAPITimeout.
	Provider domain.FlightProvider

	// RateLimiter — para rate limiting de provider (SerpAPI)
	RateLimiter *ratelimit.RateLimiter

	// Cache — la implementación de cache (normalmente cache.Dragonfly).
	// Los use cases esperan interfaces Get/Set.
	SearchCache      search_flights.Cache
	DetailsCache     flight_details.Cache
	HotelSearchCache search_hotels.Cache
	HotelDetailsCache hotel_details.Cache

	// SearchHistoryRepo — repositorio para grabar historial de búsquedas.
	// Si es nil, se crea desde PgxPool.
	Repo domain.SearchHistoryRepository

	// TTLs para cache
	SearchTTL        time.Duration
	FlightDetailsTTL time.Duration
	HotelSearchTTL   time.Duration
	HotelDetailsTTL  time.Duration

	// Pool para el repositorio de historial (solo si Repo es nil)
	PgxPool postgres.PgxPool
}

// =============================================================================
// Constructor del Módulo
// =============================================================================

// NewModule crea e inicializa el módulo Search con todas sus dependencias.
func NewModule(cfg Config) (*Module, error) {
	// 1. Provider
	provider := cfg.Provider
	if provider == nil {
		serpClient := serpapi.NewClient(cfg.SerpAPIKey, cfg.SerpAPITimeout)
		adapter := serpapi.NewAdapter(serpClient)
		if cfg.RateLimiter != nil {
			adapter.SetRateLimiter(cfg.RateLimiter)
		}
		provider = adapter
	}

	// Get the serpapi adapter reference for hotel use cases
	serpAdapter, _ := provider.(*serpapi.Adapter)

	// 2. Repository (search history)
	repo := cfg.Repo
	if repo == nil {
		repo = postgres.NewSearchHistoryRepo(cfg.PgxPool)
	}

	// 3. Use Cases — flights
	searchUC := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider:  provider,
		Cache:     cfg.SearchCache,
		Repo:      repo,
		SearchTTL: cfg.SearchTTL,
	})

	detailsUC := flight_details.NewUseCase(flight_details.UseCaseDeps{
		Provider:   provider,
		Cache:      cfg.DetailsCache,
		DetailsTTL: cfg.FlightDetailsTTL,
	})

	// 4. Use Cases — hotels
	hotelsUC := search_hotels.NewUseCase(search_hotels.UseCaseDeps{
		SerpapiAdapter: serpAdapter,
		Cache:          cfg.HotelSearchCache,
		SearchTTL:      cfg.HotelSearchTTL,
	})

	hdetailsUC := hotel_details.NewUseCase(hotel_details.UseCaseDeps{
		SerpapiAdapter: serpAdapter,
		Cache:          cfg.HotelDetailsCache,
		DetailsTTL:     cfg.HotelDetailsTTL,
	})

	// 5. Handlers
	searchHandler := search_flights.NewHandler(searchUC)
	detailsHandler := flight_details.NewHandler(detailsUC)
	hotelsHandler := search_hotels.NewHandler(hotelsUC)
	hdetailsHandler := hotel_details.NewHandler(hdetailsUC)

	// 6. Register domain error mappings
	registerSearchErrors()

	slog.Info("Search module initialized",
		"features", []string{"search_flights", "flight_details", "search_hotels", "hotel_details"},
		"search_ttl", cfg.SearchTTL,
		"details_ttl", cfg.FlightDetailsTTL,
	)

	return &Module{
		SearchFlightsHandler: searchHandler,
		FlightDetailsHandler: detailsHandler,
		SearchHotelsHandler:  hotelsHandler,
		HotelDetailsHandler:  hdetailsHandler,
		Repository:           repo,
		searchUC:             searchUC,
		detailsUC:            detailsUC,
		hotelsUC:             hotelsUC,
		hdetailsUC:           hdetailsUC,
	}, nil
}

// =============================================================================
// Constructor con Panic
// =============================================================================

// MustNewModule crea el módulo y hace panic si hay error.
func MustNewModule(cfg Config) *Module {
	mod, err := NewModule(cfg)
	if err != nil {
		panic(err)
	}
	return mod
}

// =============================================================================
// Mapeo de Errores de Dominio
// =============================================================================

func registerSearchErrors() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrInvalidTripType):
			return serrors.ErrBadRequest("Tipo de viaje no válido", err)
		case errors.Is(err, domain.ErrMissingRequiredField):
			return serrors.ErrBadRequest("Falta un campo requerido", err)
		case errors.Is(err, domain.ErrInvalidParameterRange):
			return serrors.ErrValidationError("Parámetro fuera de rango", err)
		case errors.Is(err, domain.ErrProviderUnavailable):
			return serrors.ErrServiceUnavailable("Proveedor externo no disponible", err)
		case errors.Is(err, domain.ErrProviderError):
			return serrors.ErrServiceUnavailable("Error del proveedor externo", err)
		case errors.Is(err, domain.ErrNoResults):
			return nil // empty results is valid, use case returns empty response
		case errors.Is(err, domain.ErrTokenInvalid):
			return serrors.ErrBadRequest("Token inválido o expirado", err)
		case errors.Is(err, domain.ErrTokenRequired):
			return serrors.ErrBadRequest("Token requerido", err)
		case errors.Is(err, domain.ErrCacheFailed):
			return nil // cache failure is non-fatal, let it pass through
		}
		return nil
	})
}

// =============================================================================
// TTLs por Defecto
// =============================================================================

// DefaultTTLs retorna los TTLs por defecto recomendados para las caches de búsqueda.
func DefaultTTLs() (searchTTL, detailsTTL time.Duration) {
	searchTTL = 15 * time.Minute  // resultados de búsqueda cambian frecuentemente
	detailsTTL = 15 * time.Minute // detalles de reserva también
	return
}
