// Módulo de búsqueda de vuelos — punto de entrada y wiring del módulo search.
// Expone los handlers HTTP y las dependencias necesarias.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/search/adapters/serpapi"
	"github.com/ProacTrip/Backend/internal/modules/search/consumer"
	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/execute_saved_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/flight_details"
	"github.com/ProacTrip/Backend/internal/modules/search/features/hotel_details"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	"github.com/ProacTrip/Backend/internal/modules/search/shared/airports"
	"github.com/ProacTrip/Backend/internal/modules/search/shared/conversation"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

// =============================================================================
// Estructura del Módulo
// =============================================================================

// Module expone todos los handlers y dependencias del módulo Search.
type Module struct {
	SearchFlightsHandler      *search_flights.Handler
	FlightDetailsHandler      *flight_details.Handler
	SearchHotelsHandler       *search_hotels.Handler
	HotelDetailsHandler       *hotel_details.Handler
	AISearchHandler           *ai_search.Handler
	ExecuteSavedSearchHandler *execute_saved_search.Handler
	Repository                domain.SearchHistoryRepository

	searchUC       *search_flights.UseCase
	detailsUC      *flight_details.UseCase
	hotelsUC       *search_hotels.UseCase
	hdetailsUC     *hotel_details.UseCase
	aiSearchUC     *ai_search.UseCase
	ConvConsumer   *consumer.ConversationConsumer // event-driven PG persistence
	sessionID      string                         // UUID v7 generado una vez por ciclo de vida del módulo
}

// Wait blocks until all fire-and-forget goroutines have completed.
// Call during graceful shutdown to avoid losing in-flight cache writes or history saves.
func (m *Module) Wait() {
	m.searchUC.Wait()
	m.detailsUC.Wait()
	m.hotelsUC.Wait()
	m.hdetailsUC.Wait()
	if m.aiSearchUC != nil {
		// aiSearchUC has no fire-and-forget goroutines (conversation save is synchronous)
	}
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

	// RedisClient — Dragonfly client for profile prefs / env cache lookups
	// Needed by ResolveSearchDefaults
	RedisClient *redis.Client

	// SearchDefaults — fallback defaults for GL/HL/Currency when nothing else available
	SearchDefaults shared.SearchDefaultConfig

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

	// AI Search
	AIInterpreter     domain.AIInterpreter                // AI natural language interpreter (nil = AI disabled)
	ConversationStore *conversation.PgConversationStore    // PG conversation history store

	// Event Bus — for publishing conversation_saved events to Dragonfly Streams
	EventBus *eventbus.EventBus

	// SavedSearchProvider — provides access to saved searches from the user module.
	SavedSearchProvider domain.SavedSearchProvider

	// InterpretationCacheTTL — TTL para el caché de interpretación de IA.
	// Leído de AI_INTERPRETATION_CACHE_TTL, default 10m.
	// 0 = usar default 10m en el use case.
	InterpretationCacheTTL time.Duration
}

// =============================================================================
// Constructor del Módulo
// =============================================================================

// NewModule crea e inicializa el módulo Search con todas sus dependencias.
func NewModule(cfg Config) (*Module, error) {
	// 0. Read SEARCH_CACHE_TTL from env var (default 5m) if not explicitly set.
	if cfg.SearchTTL == 0 {
		cfg.SearchTTL = 5 * time.Minute
		if envVal := os.Getenv("SEARCH_CACHE_TTL"); envVal != "" {
			if parsed, err := time.ParseDuration(envVal); err == nil {
				cfg.SearchTTL = parsed
			} else {
				slog.Warn("SEARCH_CACHE_TTL env var inválido, usando default 5m",
					slog.String("value", envVal),
					slog.String("error", err.Error()),
				)
			}
		}
	}
	// Apply SEARCH_CACHE_TTL to all cache TTLs if not explicitly set
	if cfg.FlightDetailsTTL == 0 {
		cfg.FlightDetailsTTL = cfg.SearchTTL
	}
	if cfg.HotelSearchTTL == 0 {
		cfg.HotelSearchTTL = cfg.SearchTTL
	}
	if cfg.HotelDetailsTTL == 0 {
		cfg.HotelDetailsTTL = cfg.SearchTTL
	}

	// 0a. Read AI_INTERPRETATION_CACHE_TTL from env var (default 10m) if not explicitly set.
	if cfg.InterpretationCacheTTL == 0 {
		cfg.InterpretationCacheTTL = 10 * time.Minute
		if envVal := os.Getenv("AI_INTERPRETATION_CACHE_TTL"); envVal != "" {
			if parsed, err := time.ParseDuration(envVal); err == nil {
				cfg.InterpretationCacheTTL = parsed
			} else {
				slog.Warn("AI_INTERPRETATION_CACHE_TTL env var inválido, usando default 10m",
					slog.String("value", envVal),
					slog.String("error", err.Error()),
				)
			}
		}
	}

	// 0b. Generate sessionID once per module lifecycle.
	sessionID := uuid.Must(uuid.NewV7()).String()

	// 1. Provider
	provider := cfg.Provider
	if provider == nil {
		serpClient := serpapi.NewClient(cfg.SerpAPIKey, cfg.SerpAPITimeout)
		adapter := serpapi.NewAdapter(serpClient)
		provider = adapter
	}

	// Extract HotelProvider from the same adapter instance.
	// Both FlightProvider and HotelProvider are backed by the same adapter instance.
	hotelProvider, ok := provider.(domain.HotelProvider)
	if !ok {
		return nil, errors.New("provider does not implement domain.HotelProvider")
	}

	// 2. Repository (search history)
	repo := cfg.Repo
	if repo == nil {
		repo = postgres.NewSearchHistoryRepo(cfg.PgxPool)
	}

	// 3. Use Cases — flights
	searchUC := search_flights.NewUseCase(search_flights.UseCaseDeps{
		Provider:    provider,
		Cache:       cfg.SearchCache,
		Repo:        repo,
		RateLimiter: cfg.RateLimiter,
		SearchTTL:   cfg.SearchTTL,
	})

	detailsUC := flight_details.NewUseCase(flight_details.UseCaseDeps{
		Provider:    provider,
		Cache:       cfg.DetailsCache,
		RateLimiter: cfg.RateLimiter,
		DetailsTTL:  cfg.FlightDetailsTTL,
	})

	// 4. Use Cases — hotels
	hotelsUC := search_hotels.NewUseCase(search_hotels.UseCaseDeps{
		Provider:    hotelProvider,
		Cache:       cfg.HotelSearchCache,
		RateLimiter: cfg.RateLimiter,
		SearchTTL:   cfg.HotelSearchTTL,
	})

	hdetailsUC := hotel_details.NewUseCase(hotel_details.UseCaseDeps{
		Provider:    hotelProvider,
		Cache:       cfg.HotelDetailsCache,
		RateLimiter: cfg.RateLimiter,
		DetailsTTL:  cfg.HotelDetailsTTL,
	})

	// 5. Handlers
	searchHandler := search_flights.NewHandler(searchUC, cfg.RedisClient, cfg.SearchDefaults)
	searchHandler.RateLimiter = cfg.RateLimiter
	detailsHandler := flight_details.NewHandler(detailsUC, cfg.RedisClient, cfg.SearchDefaults)
	detailsHandler.RateLimiter = cfg.RateLimiter
	hotelsHandler := search_hotels.NewHandler(hotelsUC, cfg.RedisClient, cfg.SearchDefaults)
	hotelsHandler.RateLimiter = cfg.RateLimiter
	hdetailsHandler := hotel_details.NewHandler(hdetailsUC, cfg.RedisClient, cfg.SearchDefaults)
	hdetailsHandler.RateLimiter = cfg.RateLimiter

	// 6. AI Search (nil interpreter = AI disabled — handler returns 503)
	var aiSearchHandler *ai_search.Handler
	var aiSearchUC *ai_search.UseCase
	var convConsumer *consumer.ConversationConsumer
	if cfg.AIInterpreter != nil {
		// Wire event bus for event-driven PG persistence
		if cfg.EventBus != nil && cfg.ConversationStore != nil {
			conversation.InitEventBus(cfg.EventBus)
		}

		// Wrap conversation functions as ConversationStore interface
		convStore := &dragonflyConvStore{
			rdb: cfg.RedisClient,
		}

		aiSearchUC = ai_search.NewUseCase(ai_search.UseCaseDeps{
			AIInterpreter:  cfg.AIInterpreter,
			FlightSearcher: searchUC,
			HotelSearcher:  hotelsUC,
			ConvStore:      convStore,
			InterpCache:    &dragonflyInterpCache{rdb: cfg.RedisClient},
			AnonMaxTurns:   5,
			AuthMaxTurns:   10,
			IATAResolver: func(ctx context.Context, query string) (string, error) {
				entry, err := airports.ResolveIATA(ctx, nil, query)
				if err != nil {
					return "", err
				}
				return entry.IATA, nil
			},
			RDB:         cfg.RedisClient,
			DefaultsCfg: cfg.SearchDefaults,
			// Discovery feature flags — leídos de variables de entorno
			DiscoveryEnabled:        parseEnvBool("AI_DISCOVERY_ENABLED"),
			DiscoveryClarifyEnabled: parseEnvBool("AI_DISCOVERY_CLARIFICATION_ENABLED"),
			InterpretationCacheTTL:  cfg.InterpretationCacheTTL,
		})
		aiSearchHandler = ai_search.NewHandler(aiSearchUC, cfg.RedisClient, cfg.SearchDefaults)
		aiSearchHandler.RateLimiter = cfg.RateLimiter

		// Create conversation consumer (started in bootstrap/app.go with app context)
		if cfg.EventBus != nil && cfg.ConversationStore != nil {
			convConsumer = consumer.NewConversationConsumer(cfg.RedisClient, cfg.ConversationStore)
		}
	} else {
		// Handler with nil usecase → Handle() returns 503 "AI not configured"
		aiSearchHandler = ai_search.NewHandler(nil, cfg.RedisClient, cfg.SearchDefaults)
		aiSearchHandler.RateLimiter = cfg.RateLimiter
	}

	// 7. Register domain error mappings
	registerSearchErrors()

	slog.Info("Search module initialized",
		"features", []string{"search_flights", "flight_details", "search_hotels", "hotel_details"},
		"search_ttl", cfg.SearchTTL,
		"details_ttl", cfg.FlightDetailsTTL,
		"hotel_search_ttl", cfg.HotelSearchTTL,
		"hotel_details_ttl", cfg.HotelDetailsTTL,
		"session_id", sessionID,
	)
	slog.Info("search cache TTL configured",
		slog.String("ttl", cfg.SearchTTL.String()),
	)

	// 8. Execute Saved Search — requires SavedSearchProvider to be set
	var executeSavedSearchHandler *execute_saved_search.Handler
	if cfg.SavedSearchProvider != nil {
		executeSavedSearchUC := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
			SavedSearchProvider: cfg.SavedSearchProvider,
			FlightSearcher:      searchUC,
			HotelSearcher:       hotelsUC,
			AISearcher:          aiSearchUC,
		})
		executeSavedSearchHandler = execute_saved_search.NewHandler(executeSavedSearchUC)
	}

	return &Module{
		SearchFlightsHandler:      searchHandler,
		FlightDetailsHandler:      detailsHandler,
		SearchHotelsHandler:       hotelsHandler,
		HotelDetailsHandler:       hdetailsHandler,
		AISearchHandler:           aiSearchHandler,
		ExecuteSavedSearchHandler: executeSavedSearchHandler,
		Repository:                repo,
		searchUC:             searchUC,
		detailsUC:            detailsUC,
		hotelsUC:             hotelsUC,
		hdetailsUC:           hdetailsUC,
		aiSearchUC:           aiSearchUC,
		ConvConsumer:         convConsumer,
		sessionID:            sessionID,
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
			return serrors.ErrValidationError("Falta un campo requerido", err)
		case errors.Is(err, domain.ErrInvalidParameterRange):
			return serrors.New(serrors.ProblemTypeValidationError, "Validation Error", "Parámetro fuera de rango", 422, err)
		case errors.Is(err, domain.ErrProviderUnavailable):
			return serrors.ErrServiceUnavailable("Proveedor externo no disponible", err)
		case errors.Is(err, domain.ErrProviderBadRequest):
			return serrors.ErrBadGateway("El proveedor rechazó la solicitud — parámetros inválidos", err)
		case errors.Is(err, domain.ErrProviderError):
			return serrors.ErrServiceUnavailable("Error del proveedor externo", err)
		case errors.Is(err, domain.ErrNoResults):
			return nil // handled by use cases as empty response — should never bubble to mapper
		case errors.Is(err, domain.ErrTokenInvalid):
			return serrors.ErrUnauthorized("Token inválido o expirado", err)
		case errors.Is(err, domain.ErrTokenRequired):
			return serrors.ErrBadRequest("Token requerido", err)
		case errors.Is(err, domain.ErrCacheFailed):
			return nil // cache failure is non-fatal, let it pass through
		case errors.Is(err, domain.ErrRateLimitExceeded):
			return serrors.ErrTooManyRequests("Límite de solicitudes excedido para el proveedor de búsqueda", err)
		// AI Search errors
		case errors.Is(err, domain.ErrAIUnavailable):
			return serrors.ErrServiceUnavailable("Servicio de IA no disponible", err)
		case errors.Is(err, domain.ErrAIParseFailure):
			return serrors.ErrBadGateway("La IA devolvió una respuesta inválida", err)
		case errors.Is(err, domain.ErrTurnLimitExceeded):
			return serrors.ErrBadRequest("Se alcanzó el límite máximo de turnos en la conversación", err)
		case errors.Is(err, domain.ErrConversationNotFound):
			return serrors.ErrBadRequest("Conversación no encontrada o expirada", err)
		// Conversation store (Dragonfly) failures
		case errors.Is(err, conversation.ErrConversationStoreFailed):
			return serrors.ErrServiceUnavailable("El almacenamiento de conversaciones no está disponible", err)
		// Discovery errors
		case errors.Is(err, domain.ErrDiscoveryDisabled):
			return serrors.ErrServiceUnavailable("El modo discovery no está habilitado", err)
		case errors.Is(err, domain.ErrNoCandidatesFound):
			return nil // empty response, handled by usecase — no debe llegar al mapper
		case errors.Is(err, domain.ErrClarifyMaxRounds):
			return nil // fallback a best-effort, handled by usecase — no debe llegar al mapper
		// Booking & property errors
		case errors.Is(err, domain.ErrBookingTokenExpired):
			return serrors.ErrNotFound("El token de reserva ha expirado o no es válido", err)
		case errors.Is(err, domain.ErrPropertyNotFound):
			return serrors.ErrNotFound("La propiedad no fue encontrada", err)
		}
		return nil
	})
}

// =============================================================================
// dragonflyConvStore — adapts conversation functions to ai_search.ConversationStore
// =============================================================================

// dragonflyConvStore implements ai_search.ConversationStore by wrapping
// the conversation package's Dragonfly-backed functions.
type dragonflyConvStore struct {
	rdb *redis.Client
}

func (s *dragonflyConvStore) GetConversation(ctx context.Context, id string) (*domain.ConversationState, error) {
	return conversation.GetConversation(ctx, s.rdb, id)
}

func (s *dragonflyConvStore) SaveConversation(ctx context.Context, conv *domain.ConversationState) error {
	return conversation.SaveConversation(ctx, s.rdb, conv)
}

// =============================================================================
// dragonflyInterpCache — DragonflyDB-backed AI interpretation cache
// =============================================================================

// dragonflyInterpCache implements ai_search.InterpretationCache using DragonflyDB.
// Keys are blake3 hashes of (message + conversation history).
// Only complete intents ("flights", "hotels", "both") are cached with 10 min TTL.
type dragonflyInterpCache struct {
	rdb *redis.Client
}

// Get retrieves a cached TravelIntent by blake3 hash key.
// Returns redis.Nil if the key does not exist (cache miss).
func (c *dragonflyInterpCache) Get(ctx context.Context, key string) (*domain.TravelIntent, error) {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var intent domain.TravelIntent
	if err := json.Unmarshal(raw, &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}

// Set stores a TravelIntent in the cache with the given TTL.
// Marshal errors are returned; Dragonfly write errors are returned as-is.
func (c *dragonflyInterpCache) Set(ctx context.Context, key string, intent *domain.TravelIntent, ttl time.Duration) error {
	raw, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, raw, ttl).Err()
}

// =============================================================================
// Feature flags — lectura de variables de entorno
// =============================================================================

// parseEnvBool lee una variable de entorno como booleano.
// "true" y "1" → true, cualquier otro valor → false.
func parseEnvBool(key string) bool {
	val := os.Getenv(key)
	switch val {
	case "true", "1":
		return true
	default:
		return false
	}
}
