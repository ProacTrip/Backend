// Lógica de negocio para AI-powered unified search.
// Orquesta interpretación de lenguaje natural y ejecución de búsquedas.
//
// Flujo:
//  1. Get or create conversation (Dragonfly)
//  2. Check turn limits (anon=5, auth=10)
//  3. Call AIInterpreter.Parse(message, history)
//  4. Switch on intent.Type → execute flight/hotel/both searchers
//  5. Apply FilterCriteria (deterministic Go) if present
//  6. Save/update conversation state
//  7. Save PG history if auth user (fire-and-forget)
//  8. Return unified response
package ai_search

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"lukechampine.com/blake3"

	envDomain "github.com/ProacTrip/Backend/internal/modules/environment/domain"
	"github.com/ProacTrip/Backend/internal/modules/environment/features/get_destination_weather"
	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
	searchshared "github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	"github.com/ProacTrip/Backend/internal/modules/search/shared/airports"
	sharedEnv "github.com/ProacTrip/Backend/internal/shared/environment"
	"golang.org/x/sync/errgroup"
)

// =============================================================================
// Dependency interfaces for testability
// =============================================================================

// FlightSearcher is the interface for flight search execution.
// search_flights.UseCase satisfies this.
type FlightSearcher interface {
	Execute(ctx context.Context, cmd search_flights.Command) (*search_flights.Response, error)
}

// HotelSearcher is the interface for hotel search execution.
// search_hotels.UseCase satisfies this.
type HotelSearcher interface {
	Execute(ctx context.Context, cmd search_hotels.Command) (*search_hotels.Response, error)
}

// DestinationWeatherSearcher is the interface for destination weather queries.
// get_destination_weather.UseCase satisfies this.
type DestinationWeatherSearcher interface {
	Execute(ctx context.Context, cmd get_destination_weather.Command) (*envDomain.WeatherData, error)
}

// ConversationStore abstracts conversation persistence.
type ConversationStore interface {
	// Legacy methods — return domain.ConversationState (used by runExactSearch).
	GetConversation(ctx context.Context, id string) (*domain.ConversationState, error)
	SaveConversation(ctx context.Context, conv *domain.ConversationState) error

	// New methods — return ai_search.Conversation (used by chat streaming + handlers).
	Save(ctx context.Context, conv *Conversation) error
	Load(ctx context.Context, convID string) (*Conversation, error)
	Delete(ctx context.Context, convID, userID string) error
	ListUserConversations(ctx context.Context, userID string) ([]ConversationPreview, error)
	ResetTTL(ctx context.Context, convID string) error
}

// InterpretationCache is the cache interface for AI interpretation results.
// Keys are blake3 hashes of (message + history). Only complete intents
// ("flights", "hotels", "both") are cached; incomplete/ambiguous queries
// always go to the AI interpreter for fresh follow-up questions.
type InterpretationCache interface {
	Get(ctx context.Context, key string) (*domain.TravelIntent, error)
	Set(ctx context.Context, key string, intent *domain.TravelIntent, ttl time.Duration) error
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orchestrates AI-powered unified search.
type UseCase struct {
	interpreter          domain.AIInterpreter
	discoveryInterpreter domain.DiscoveryInterpreter // AI-powered discovery (nil = disabled)
	flightSearcher       FlightSearcher
	hotelSearcher        HotelSearcher
	dstWeatherSearcher   DestinationWeatherSearcher
	convStore            ConversationStore
	interpCache          InterpretationCache
	toolCallStreamer     domain.ToolCallStreamer      // streaming with tool calling
	iataResolver         func(ctx context.Context, query string) (string, error)
	rdb                  *redis.Client                    // Dragonfly for location hint resolution (env:{ip}, profile:{userID}:prefs)
	defaultsCfg          searchshared.SearchDefaultConfig // Fallback defaults (DEFAULT_COUNTRY_CODE, etc.)
	anonMaxTurns         int
	authMaxTurns         int

	// interpCacheTTL es el TTL configurable para el caché de interpretación de IA.
	// Default 10*time.Minute si no se configura vía UseCaseDeps.InterpretationCacheTTL.
	interpCacheTTL time.Duration
}

// UseCaseDeps bundles dependencies for the AI search use case.
type UseCaseDeps struct {
	AIInterpreter          domain.AIInterpreter
	DiscoveryInterpreter   domain.DiscoveryInterpreter // nil = discovery disabled
	FlightSearcher         FlightSearcher
	HotelSearcher          HotelSearcher
	DstWeatherSearcher     DestinationWeatherSearcher   // nil = weather tool disabled
	ConvStore              ConversationStore
	InterpCache            InterpretationCache          // nil = no caching (MVP mode)
	ToolCallStreamer       domain.ToolCallStreamer      // streaming with tool calling
	AnonMaxTurns           int
	AuthMaxTurns           int

	// IATAResolver resolves city names or partial airport identifiers to IATA codes.
	// Used to normalize AI-extracted params (e.g., "Madrid" → "MAD").
	// nil skips IATA normalization (useful for tests that don't need it).
	IATAResolver func(ctx context.Context, query string) (string, error)

	// RDB is the Dragonfly client for location hint resolution (env:{ip}, profile:{userID}:prefs).
	// nil disables location hints (pass nil in tests that don't need location context).
	RDB *redis.Client

	// DefaultsCfg are the hardcoded fallback defaults (DEFAULT_COUNTRY_CODE, DEFAULT_LANGUAGE, DEFAULT_CURRENCY)
	// from environment config. Used as fallback when Dragonfly env:{ip} cache doesn't exist (first request, local testing).
	DefaultsCfg searchshared.SearchDefaultConfig

	// InterpretationCacheTTL es el TTL para el caché de interpretación de IA.
	// Leído de la variable de entorno AI_INTERPRETATION_CACHE_TTL, default 10m.
	// Si es 0, se usa 10*time.Minute.
	InterpretationCacheTTL time.Duration
}

// NewUseCase creates a new AI search use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	if deps.AnonMaxTurns <= 0 {
		deps.AnonMaxTurns = 5
	}
	if deps.AuthMaxTurns <= 0 {
		deps.AuthMaxTurns = 10
	}
	// Default 10m si no se configuró explícitamente.
	interpCacheTTL := deps.InterpretationCacheTTL
	if interpCacheTTL <= 0 {
		interpCacheTTL = 10 * time.Minute
	}

	return &UseCase{
		interpreter:          deps.AIInterpreter,
		discoveryInterpreter: deps.DiscoveryInterpreter,
		flightSearcher:       deps.FlightSearcher,
		hotelSearcher:        deps.HotelSearcher,
		dstWeatherSearcher:   deps.DstWeatherSearcher,
		convStore:            deps.ConvStore,
		interpCache:          deps.InterpCache,
		toolCallStreamer:     deps.ToolCallStreamer,
		iataResolver:         deps.IATAResolver,
		rdb:                  deps.RDB,
		defaultsCfg:          deps.DefaultsCfg,
		anonMaxTurns:         deps.AnonMaxTurns,
		authMaxTurns:         deps.AuthMaxTurns,
		interpCacheTTL:       interpCacheTTL,
	}
}

// =============================================================================
// Execute — main orchestration
// =============================================================================

// Execute orchestrates the AI interpretation and search execution.
// userID is empty for anonymous users.
//
// Dispatch: si DiscoveryEnabled y la consulta es de discovery (o el hint lo fuerza),
// ejecuta el pipeline de discovery. Si no, ejecuta el flujo exact search existente.
func (uc *UseCase) Execute(ctx context.Context, cmd Command, userID string) (*Response, error) {
	// 1. Validate command
	if err := cmd.Validate(); err != nil {
		slog.WarnContext(ctx, "ai_search.Execute: command validation failed",
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	slog.DebugContext(ctx, "ai_search.Execute: start",
		slog.String("message", cmd.Message[:min(len(cmd.Message), 80)]),
		slog.String("conversation_id", cmd.ConversationID),
		slog.String("user_id", userID),
	)

	// 2. Route to discovery pipeline when available (REQ-W1).
	// Discovery handles open-ended queries without specific flight/hotel parameters.
	// Only applies to first messages (no conversation_id) — follow-ups
	// always go through the exact search pipeline to continue the conversation.
	// Queries with dates, IATA codes, or explicit flight/hotel terms are also excluded
	// by isDiscoveryQuery.
	if uc.discoveryInterpreter != nil && cmd.ConversationID == "" && isDiscoveryQuery(cmd.Message) {
		return uc.runDiscovery(ctx, cmd, userID)
	}

	// 3. Default: exact search (existing behavior)
	return uc.runExactSearch(ctx, cmd, userID)
}

// =============================================================================
// runExactSearch — flujo de búsqueda exacta (existente)
// =============================================================================

// runExactSearch ejecuta el flujo de búsqueda exacta existente:
// conversation management, AI interpretation, flight/hotel search execution.
func (uc *UseCase) runExactSearch(ctx context.Context, cmd Command, userID string) (*Response, error) {
	// Nil guard: si no hay interpreter, no se puede ejecutar exact search
	if uc.interpreter == nil {
		return nil, fmt.Errorf("ai interpreter: %w", domain.ErrAIUnavailable)
	}

	// 2. Get or create conversation
	conv, err := uc.getOrCreateConversation(ctx, cmd.ConversationID, userID)
	if err != nil {
		slog.ErrorContext(ctx, "ai_search: conversation store failed",
			slog.String("conversation_id", cmd.ConversationID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("conversation store: %w", err)
	}

	// 3. Check turn limits
	maxTurns := uc.maxTurnsForUser(userID)
	if conv.TurnCount >= maxTurns {
		slog.WarnContext(ctx, "ai_search.Execute: turn limit exceeded",
			slog.String("conversation_id", conv.ID),
			slog.Int("turn_count", conv.TurnCount),
			slog.Int("max_turns", maxTurns),
		)
		return nil, fmt.Errorf("turn %d/%d: %w", conv.TurnCount, maxTurns, domain.ErrTurnLimitExceeded)
	}

	slog.DebugContext(ctx, "ai_search.Execute: calling AI interpreter",
		slog.String("conversation_id", conv.ID),
		slog.Int("history_len", len(conv.Messages)),
		slog.Int("turn_count", conv.TurnCount),
	)

	// 3b. Inject location hint as system context for anonymous/auth users
	// so the AI assumes the user's detected city as default departure.
	if hint := uc.resolveLocationHint(ctx, userID, cmd.ClientIP); hint != "" {
		conv.Messages = prependSystemContext(conv.Messages, hint)
	}

	// 4. Call AI interpreter (with blake3 cache for complete intents)
	intent, fromCache, err := uc.interpretWithCache(ctx, cmd.Message, conv.Messages, cmd.HL)
	if err != nil {
		if errors.Is(err, domain.ErrAIParseFailure) {
			slog.ErrorContext(ctx, "ai_search: AI parse failure",
				slog.String("conversation_id", conv.ID),
				slog.String("message", cmd.Message),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("ai interpreter: %w", domain.ErrAIParseFailure)
		}
		slog.ErrorContext(ctx, "ai_search: AI interpreter unavailable",
			slog.String("conversation_id", conv.ID),
			slog.String("message", cmd.Message),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("ai interpreter: %w", domain.ErrAIUnavailable)
	}

	slog.DebugContext(ctx, "ai_search.Execute: AI intent resolved",
		slog.String("conversation_id", conv.ID),
		slog.String("intent_type", intent.Type),
		slog.Float64("confidence", intent.Confidence),
	)

	// 4b. Normalize AI-extracted params before passing to searchers.
	// The AI often returns city names ("Madrid") or numeric values (stops=0)
	// instead of IATA codes and valid string enums expected by SerpAPI.
	uc.normalizeIntent(ctx, intent)

	// 5. Build conversation messages
	userMsg := domain.ConversationMessage{
		Role:      "user",
		Content:   cmd.Message,
		Timestamp: time.Now(),
	}
	assistantMsg := domain.ConversationMessage{
		Role:      "assistant",
		Timestamp: time.Now(),
	}

	// 6. Switch on intent type
	var flightResp *search_flights.Response
	var hotelResp *search_hotels.Response
	var flightErr, hotelErr error // partial error tracking for "both" intent

	switch intent.Type {
	case "incomplete", "ambiguous":
		// No search — just set the follow-up message
		assistantMsg.Content = intent.FollowUp

	case "flights":
		flightCmd := buildFlightCommand(intent, cmd)
		flightResp, err = uc.flightSearcher.Execute(ctx, flightCmd)
		if err != nil {
			if errors.Is(err, domain.ErrNoResults) {
				flightResp = &search_flights.Response{ResultsState: "empty"}
			} else {
				return nil, fmt.Errorf("flight search: %w", err)
			}
		}
		assistantMsg.Content = generateFlightsMessage(intent, flightResp)

	case "hotels":
		hotelCmd := buildHotelCommand(intent, cmd)
		hotelResp, err = uc.hotelSearcher.Execute(ctx, hotelCmd)
		if err != nil {
			if errors.Is(err, domain.ErrNoResults) {
				hotelResp = &search_hotels.Response{ResultsState: "empty"}
			} else {
				return nil, fmt.Errorf("hotel search: %w", err)
			}
		}
		assistantMsg.Content = generateHotelsMessage(intent, hotelResp)

	case "both":
		flightCmd := buildFlightCommand(intent, cmd)
		hotelCmd := buildHotelCommand(intent, cmd)

		// Collect results independently with mutex — partial results
		// are returned when one searcher fails and the other succeeds.
		var mu sync.Mutex

		g := new(errgroup.Group)
		g.Go(func() error {
			var fResp *search_flights.Response
			var fErr error
			fResp, fErr = uc.flightSearcher.Execute(ctx, flightCmd)
			// ErrNoResults is not a real error — convert to empty response
			if errors.Is(fErr, domain.ErrNoResults) {
				fResp = &search_flights.Response{ResultsState: "empty"}
				fErr = nil
			}
			mu.Lock()
			flightResp = fResp
			flightErr = fErr
			mu.Unlock()
			return nil // NEVER return error from goroutine — collect manually
		})
		g.Go(func() error {
			var hResp *search_hotels.Response
			var hErr error
			hResp, hErr = uc.hotelSearcher.Execute(ctx, hotelCmd)
			// ErrNoResults is not a real error — convert to empty response
			if errors.Is(hErr, domain.ErrNoResults) {
				hResp = &search_hotels.Response{ResultsState: "empty"}
				hErr = nil
			}
			mu.Lock()
			hotelResp = hResp
			hotelErr = hErr
			mu.Unlock()
			return nil // NEVER return error from goroutine — collect manually
		})
		g.Wait()

		// Both failed → fatal, no partial results possible.
		// Wrap with ErrProviderUnavailable so error mapper returns 502 Bad Gateway
		// instead of 500 Internal Server Error.
		if flightErr != nil && hotelErr != nil {
			return nil, fmt.Errorf("%w: flights: %w | hotels: %w",
				domain.ErrProviderUnavailable, flightErr, hotelErr)
		}

		// Build partial/mixed response message
		switch {
		case flightErr != nil && hotelErr == nil:
			assistantMsg.Content = generateHotelsMessage(intent, hotelResp) +
				" Los vuelos no se pudieron obtener en este momento."
		case hotelErr != nil && flightErr == nil:
			assistantMsg.Content = generateFlightsMessage(intent, flightResp) +
				" Los hoteles no se pudieron obtener en este momento."
		default:
			// Both succeeded — check if results are empty vs. populated.
			flightsCount := 0
			if flightResp != nil {
				flightsCount = len(flightResp.BestFlights) + len(flightResp.OtherFlights)
			}
			hotelsCount := 0
			if hotelResp != nil {
				hotelsCount = len(hotelResp.Properties)
			}
			if flightsCount == 0 && hotelsCount == 0 {
				assistantMsg.Content = "No se encontraron resultados para tus criterios de búsqueda. ¿Querés ajustar algo?"
			} else if flightsCount > 0 && hotelsCount > 0 {
				assistantMsg.Content = "Acá tenés los resultados de vuelos y hoteles que encontré."
			} else if flightsCount > 0 {
				assistantMsg.Content = generateFlightsMessage(intent, flightResp) +
					" No encontré alojamientos para esos criterios."
			} else {
				assistantMsg.Content = generateHotelsMessage(intent, hotelResp) +
					" No encontré vuelos para esos criterios."
			}
		}

	default:
		// Unknown intent type → treat as ambiguous
		assistantMsg.Content = "No entendí bien tu consulta. ¿Podrías darme más detalles sobre tu viaje?"
		intent.Type = "ambiguous"
	}

	slog.DebugContext(ctx, "ai_search.Execute: search execution complete",
		slog.String("conversation_id", conv.ID),
		slog.String("intent_type", intent.Type),
		slog.Bool("has_flight_results", flightResp != nil),
		slog.Bool("has_hotel_results", hotelResp != nil),
		slog.Any("flight_err", flightErr),
		slog.Any("hotel_err", hotelErr),
	)

	// 7. Update conversation — strip injected system messages before saving
	// so they don't accumulate across turns (they're re-injected each turn).
	conv.Messages = append(stripSystemMessages(conv.Messages), userMsg, assistantMsg)
	conv.Intent = intent
	conv.TurnCount++

	// Store results as JSON RawMessage
	if flightResp != nil || hotelResp != nil {
		combined := struct {
			Flights *search_flights.Response `json:"flights,omitzero"`
			Hotels  *search_hotels.Response  `json:"hotels,omitzero"`
		}{
			Flights: flightResp,
			Hotels:  hotelResp,
		}
		resultsJSON, err := json.Marshal(combined)
		if err != nil {
			// Non-fatal: log and skip storing results (response already has separate fields).
			slog.ErrorContext(ctx, "ai_search: failed to marshal combined results",
				slog.String("conversation_id", conv.ID),
				slog.String("error", err.Error()),
			)
		} else {
			conv.Results = resultsJSON
		}
	}

	if saveErr := uc.convStore.SaveConversation(ctx, conv); saveErr != nil {
		// Non-fatal: log the error so operators can diagnose Dragonfly issues,
		// but don't fail the request — the user already got their AI response.
		slog.ErrorContext(ctx, "ai_search: failed to save conversation",
			slog.String("conversation_id", conv.ID),
			slog.Int("turn_count", conv.TurnCount),
			slog.String("error", saveErr.Error()),
		)
	} else {
		slog.DebugContext(ctx, "ai_search.Execute: conversation saved",
			slog.String("conversation_id", conv.ID),
			slog.Int("turn_count", conv.TurnCount),
		)
	}

	// Also save in the new Conversation format ({conv}:{id}) so that
	// ListUserConversations and HandleGetConversation can retrieve it.
	// This dual-write ensures backward compatibility while maintaining
	// the user conversation index (user:convs:{userID}) via SADD.
	// Build SearchCache from the flight/hotel responses so the frontend
	// can restore results when loading a previous conversation.
	searchCache := make(map[string]*CachedSearch)
	if flightResp != nil {
		flightJSON, _ := json.Marshal(flightResp)
		searchCache["flights"] = &CachedSearch{
			Response: flightJSON,
			Type:     "flights",
		}
	}
	if hotelResp != nil {
		hotelJSON, _ := json.Marshal(hotelResp)
		searchCache["hotels"] = &CachedSearch{
			Response: hotelJSON,
			Type:     "hotels",
		}
	}
	if saveErr := uc.convStore.Save(ctx, &Conversation{
		ID:          conv.ID,
		UserID:      conv.UserID,
		Messages:    conv.Messages,
		SearchCache: searchCache,
		TurnCount:   conv.TurnCount,
		MaxTurns:    conv.MaxTurns,
		CreatedAt:   conv.CreatedAt,
	}); saveErr != nil {
		slog.ErrorContext(ctx, "ai_search: failed to save new-format conversation",
			slog.String("conversation_id", conv.ID),
			slog.String("error", saveErr.Error()),
		)
	}

	// 8. Build response
	slog.DebugContext(ctx, "ai_search.Execute: building response",
		slog.String("conversation_id", conv.ID),
		slog.Int("turn_count", conv.TurnCount),
		slog.String("intent_type", intent.Type),
	)
	resp := &Response{
		ConversationID: conv.ID,
		TurnCount:      conv.TurnCount,
		MaxTurns:       maxTurns,
		Intent:         intent.Type,
		Confidence:     intent.Confidence,
		Message:        assistantMsg.Content,
		MissingFields:  intent.MissingFields,
		FromCache:      fromCache,
	}

	// Marshal flight/hotel responses into RawMessage
	if flightResp != nil {
		if data, err := json.Marshal(flightResp); err == nil {
			resp.Flights = json.RawMessage(data)
		}
	}
	if hotelResp != nil {
		if data, err := json.Marshal(hotelResp); err == nil {
			resp.Hotels = json.RawMessage(data)
		}
	}
	// Error markers for partial failures ("both" intent)
	if flightErr != nil {
		resp.FlightsError = flightErr.Error()
	}
	if hotelErr != nil {
		resp.HotelsError = hotelErr.Error()
	}

	return resp, nil
}

// runDiscovery — AI-powered discovery pipeline
// =============================================================================

// runDiscovery ejecuta el pipeline de discovery usando DeepSeek v4 Flash.
// Construye el DiscoveryContext con datos de ubicación y preferencias del usuario,
// llama al DiscoveryInterpreter, y devuelve la respuesta formateada.
func (uc *UseCase) runDiscovery(ctx context.Context, cmd Command, userID string) (*Response, error) {
	slog.InfoContext(ctx, "ai_search: discovery request",
		slog.String("query", cmd.Message),
		slog.String("user_id", userID),
	)

	// Build DiscoveryContext from resolved defaults + current date.
	// Location is resolved from env:{ip} cache inside the usecase.
	discCtx := domain.DiscoveryContext{
		Currency: cmd.Currency,
		Language: cmp.Or(cmd.HL, "es"),
		Date:     time.Now().Format("2006-01-02"),
	}

	// Resolve location from env:{ip} cache
	if uc.rdb != nil && cmd.ClientIP != "" {
		if entry := uc.resolveEnvCacheEntry(ctx, cmd.ClientIP); entry != nil {
			discCtx.CountryCode = entry.Location.CountryCode
			discCtx.Timezone = entry.Location.Timezone
			discCtx.Lat = entry.Location.Latitude
			discCtx.Lng = entry.Location.Longitude
		}
	}

	// Fallback language
	if discCtx.Language == "" {
		discCtx.Language = "es"
	}

	// Create a conversation for the discovery session so the response
	// includes conversation_id (matching exact search response structure).
	convID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate conversation ID: %w", err)
	}
	maxTurns := uc.maxTurnsForUser(userID)

	// Call the AI discovery interpreter with conversation history (empty for now).
	// The AI returns natural language text with recommendations.
	aiMessage, err := uc.discoveryInterpreter.Discover(ctx, cmd.Message, discCtx, nil)
	if err != nil {
		slog.ErrorContext(ctx, "ai_search: discovery AI call failed",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("discovery interpreter: %w", domain.ErrAIUnavailable)
	}

	slog.InfoContext(ctx, "ai_search: discovery response",
		slog.String("query", cmd.Message),
		slog.Int("response_len", len(aiMessage)),
	)

	// Build response with the AI's natural language message.
	// Includes conversation_id, turn_count, and max_turns to match the exact
	// search response structure expected by the frontend.
	resp := &Response{
		ConversationID: convID.String(),
		TurnCount:      1,
		MaxTurns:       maxTurns,
		Mode:           "discovery",
		Intent:         string(SearchModeDiscovery),
		Confidence:     1.0, // AI-handled, always confident
		Message:        aiMessage,
		FromCache:      false,
	}

	// Save to ConvStore so discovery conversations appear in GET /conversations.
	// Uses the same {conv}:{id} dual-write pattern as ExecuteChatStream.
	now := time.Now()
	userMsg := domain.ConversationMessage{
		Role:      "user",
		Content:   cmd.Message,
		Timestamp: now,
	}
	assistantMsg := domain.ConversationMessage{
		Role:      "assistant",
		Content:   aiMessage,
		Timestamp: now,
	}
	// Save to LEGACY store (ConversationState) so getOrCreateConversation
	// can find it when the user sends a follow-up message. Without this,
	// each follow-up starts a new conversation with zero context.
	if saveErr := uc.convStore.SaveConversation(ctx, &domain.ConversationState{
		ID:        convID.String(),
		UserID:    userID,
		Messages:  []domain.ConversationMessage{userMsg, assistantMsg},
		TurnCount: 1,
		MaxTurns:  maxTurns,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	}); saveErr != nil {
		slog.ErrorContext(ctx, "ai_search: failed to save discovery conversation (legacy)",
			slog.String("conversation_id", convID.String()),
			slog.String("error", saveErr.Error()),
		)
	}

	// Also save to the new Conversation format ({conv}:{id}) so
	// HandleGetConversation and conversation history work correctly.
	if saveErr := uc.convStore.Save(ctx, &Conversation{
		ID:        convID.String(),
		UserID:    userID,
		Messages:  []domain.ConversationMessage{userMsg, assistantMsg},
		TurnCount: 1,
		MaxTurns:  maxTurns,
		CreatedAt: now,
	}); saveErr != nil {
		slog.ErrorContext(ctx, "ai_search: failed to save discovery conversation to ConvStore",
			slog.String("conversation_id", convID.String()),
			slog.String("error", saveErr.Error()),
		)
	}

	return resp, nil
}

// resolveEnvCacheEntry reads the full env:{ip} cache entry from DragonflyDB.
// Returns nil if the cache is not available or cannot be parsed.
func (uc *UseCase) resolveEnvCacheEntry(ctx context.Context, ip string) *sharedEnv.CacheEntry {
	if uc.rdb == nil {
		return nil
	}
	key := sharedEnv.CacheKey(ip)
	raw, err := uc.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil
	}
	if raw == "" {
		return nil
	}
	var entry sharedEnv.CacheEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		return nil
	}
	return &entry
}

// =============================================================================
// Conversation management
// =============================================================================

// getOrCreateConversation retrieves an existing conversation or creates a new one.
//
// Behaviour:
//   - conversationID empty → creates a NEW conversation (first request)
//   - conversationID provided AND found → returns existing conversation
//   - conversationID provided but NOT found → returns ErrConversationNotFound (400)
//     Never silently creates a new conversation for an invalid ID.
func (uc *UseCase) getOrCreateConversation(ctx context.Context, conversationID string, userID string) (*domain.ConversationState, error) {
	if conversationID != "" {
		conv, err := uc.convStore.GetConversation(ctx, conversationID)
		if err != nil {
			slog.ErrorContext(ctx, "getOrCreateConversation: GetConversation failed",
				slog.String("conversation_id", conversationID),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("get conversation %s: %w", conversationID, err)
		}
		if conv != nil {
			slog.DebugContext(ctx, "getOrCreateConversation: found existing conversation",
				slog.String("conversation_id", conv.ID),
				slog.Int("turn_count", conv.TurnCount),
			)
			return conv, nil
		}
		// Conversation ID provided but not found → invalid/expired ID
		slog.WarnContext(ctx, "getOrCreateConversation: conversation not found",
			slog.String("conversation_id", conversationID),
		)
		return nil, domain.ErrConversationNotFound
	}

	// No conversation ID → first request, create new conversation
	convID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate conversation ID: %w", err)
	}

	maxTurns := uc.maxTurnsForUser(userID)
	now := time.Now()

	conv := &domain.ConversationState{
		ID:        convID.String(),
		UserID:    userID,
		Messages:  []domain.ConversationMessage{},
		TurnCount: 0,
		MaxTurns:  maxTurns,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	}

	slog.DebugContext(ctx, "getOrCreateConversation: created new conversation",
		slog.String("conversation_id", conv.ID),
		slog.String("user_id", userID),
	)

	return conv, nil
}

// maxTurnsForUser returns the max turns for a given user type.
func (uc *UseCase) maxTurnsForUser(userID string) int {
	if userID != "" {
		return uc.authMaxTurns
	}
	return uc.anonMaxTurns
}

// isDiscoveryQuery checks whether a message is likely a discovery query
// (open-ended, no specific flight/hotel search parameters).
// Returns true for queries like "recomiéndame playas" or "destinos baratos en Europa".
// Returns false for queries with dates, IATA codes, or explicit flight/hotel terms.
//
// The AI interpreter doesn't yet classify "discovery" as a distinct intent type,
// so this heuristic gates routing to the discovery pipeline (REQ-W1).
func isDiscoveryQuery(message string) bool {
	lower := strings.ToLower(message)

	// Explicit date patterns → exact search
	if strings.Contains(lower, "202") || strings.Contains(lower, "enero") ||
		strings.Contains(lower, "febrero") || strings.Contains(lower, "marzo") ||
		strings.Contains(lower, "abril") || strings.Contains(lower, "mayo") ||
		strings.Contains(lower, "junio") || strings.Contains(lower, "julio") ||
		strings.Contains(lower, "agosto") || strings.Contains(lower, "septiembre") ||
		strings.Contains(lower, "octubre") || strings.Contains(lower, "noviembre") ||
		strings.Contains(lower, "diciembre") ||
		strings.Contains(lower, "january") || strings.Contains(lower, "february") ||
		strings.Contains(lower, "march") || strings.Contains(lower, "april") ||
		strings.Contains(lower, "june") || strings.Contains(lower, "july") ||
		strings.Contains(lower, "august") || strings.Contains(lower, "september") ||
		strings.Contains(lower, "october") || strings.Contains(lower, "november") ||
		strings.Contains(lower, "december") {
		return false
	}

	// Flight-specific terms → exact search
	if strings.Contains(lower, "vuelo") || strings.Contains(lower, "vuelos") ||
		strings.Contains(lower, "pasaje") || strings.Contains(lower, "pasajes") ||
		strings.Contains(lower, "ida y vuelta") || strings.Contains(lower, "solo ida") ||
		strings.Contains(lower, "escala") {
		return false
	}

	// Hotel-specific terms → exact search
	if strings.Contains(lower, "hotel") || strings.Contains(lower, "alojamiento") ||
		strings.Contains(lower, "habitación") || strings.Contains(lower, "estadia") {
		return false
	}

	// Discovery keywords → discovery pipeline
	discoveryKeywords := []string{
		"recomiéndame", "recomiendame", "recomienda", "recomendar",
		"descubrir", "descubriendo", "conocer",
		"destino", "destinos", "viaje", "viajes", "viajar",
		"vacaciones", "escapada", "turismo",
		"playa", "playas", "montaña", "montañas",
		"barato", "baratos", "económico", "económicos",
		"mejor época", "cuando ir", "clima",
	}
	for _, kw := range discoveryKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// Very short queries without search terms → discovery
	if len(strings.Fields(message)) <= 3 {
		return true
	}

	return false
}

// =============================================================================
// Location hint — injects detected user location as AI context
// =============================================================================

// The env:{ip} cache contract is defined in shared/environment/dto.go.
// We use sharedEnv.LocationDTO to decode location data from the cache.

// resolveLocationHint determines the user's location for AI context injection.
//
// Resolution strategy:
//   - Both auth and anonymous users: parse the env:{ip} Dragonfly cache
//     (full EnvironmentResponse JSON from /v1/environment) to extract
//     city, country, and country_code. IATA airport code is resolved
//     from city or country name.
//   - If no data is available, returns "" (no hint injected).
//   - CountryCode is NOT read from profile prefs — that's the user's
//     nationality, not their current location.
//
// The returned string is a Spanish system context message instructing the AI
// to use the detected location as default departure for flight searches.
func (uc *UseCase) resolveLocationHint(ctx context.Context, userID, clientIP string) string {
	if uc.rdb == nil {
		slog.DebugContext(ctx, "resolveLocationHint: no rdb configured, skipping")
		return ""
	}

	var city, country, countryCode, iata string

	// Both auth and anonymous users resolve location from env:{ip} cache.
	// CountryCode is NO LONGER read from profile prefs (Phase 2 ai-discovery-rewrite).
	if clientIP != "" {
		key := sharedEnv.CacheKey(clientIP)
		raw, err := uc.rdb.Get(ctx, key).Result()
		if err != nil {
			if err != redis.Nil {
				slog.WarnContext(ctx, "resolveLocationHint: env cache lookup failed",
					slog.String("ip", clientIP),
					slog.String("error", err.Error()),
				)
			} else {
				slog.DebugContext(ctx, "resolveLocationHint: env cache miss, trying fallback",
					slog.String("ip", clientIP),
				)
			}
			// Fall through to default fallback below
		}
		if raw != "" {
			var cacheEntry sharedEnv.CacheEntry
			if err := json.Unmarshal([]byte(raw), &cacheEntry); err != nil {
				slog.WarnContext(ctx, "resolveLocationHint: env cache unmarshal failed",
					slog.String("error", err.Error()),
				)
			} else {
				city = cacheEntry.Location.City
				country = cacheEntry.Location.Country
				countryCode = cacheEntry.Location.CountryCode
			}
		}

		// If cache provided usable data, resolve IATA.
		if city != "" || country != "" || countryCode != "" {
			// Try to resolve IATA code for the city.
			if city != "" {
				if entry, err := airports.ResolveIATA(ctx, nil, city); err == nil && entry != nil {
					iata = entry.IATA
				}
			}
			// Fallback: country → IATA if city resolution failed or city is empty.
			if iata == "" && country != "" {
				if mainIATA, found := airports.ResolveCountryToIATA(country); found {
					iata = mainIATA
				}
			}
		} else {
			// No usable data from env cache — fall through to default fallback.
			slog.DebugContext(ctx, "resolveLocationHint: env cache empty/insufficient, trying default fallback",
				slog.String("ip", clientIP),
			)
		}
	}

	// NO fallback: if IP-based location resolution fails, return empty hint.
	// The AI will ask "¿desde dónde?" instead of assuming a wrong location.
	// DEFAULT_COUNTRY_CODE is a platform currency/pricing default, NOT a user location.

	if city == "" && country == "" && countryCode == "" {
		slog.DebugContext(ctx, "resolveLocationHint: no location data available")
		return ""
	}

	hint := buildLocationHint(city, country, countryCode, iata)
	slog.DebugContext(ctx, "resolveLocationHint: hint built successfully",
		slog.String("city", city),
		slog.String("country", country),
		slog.String("country_code", countryCode),
		slog.String("iata", iata),
		slog.String("hint_preview", hint[:min(len(hint), 100)]),
	)
	return hint
}

// buildLocationHint formats the detected location into an AI system context message.
// The message instructs the AI to use the detected location as default departure
// for flight searches, avoiding the generic "¿Desde dónde?" question.
func buildLocationHint(city, country, countryCode, iata string) string {
	// Build the location descriptor
	var location string
	switch {
	case city != "" && country != "":
		location = fmt.Sprintf("%s, %s", city, country)
	case country != "":
		location = country
	default:
		location = countryCode
	}

	// Build IATA / country code suffix
	var suffix string
	switch {
	case iata != "" && countryCode != "":
		suffix = fmt.Sprintf(" (IATA: %s, código de país: %s)", iata, countryCode)
	case iata != "":
		suffix = fmt.Sprintf(" (IATA: %s)", iata)
	case countryCode != "":
		suffix = fmt.Sprintf(" (código de país: %s)", countryCode)
	}

	return fmt.Sprintf(
		"Contexto: La ubicación detectada del usuario es %s%s. "+
			"Usa esto como origen por defecto para búsquedas de vuelos a menos que "+
			"el usuario especifique otra ciudad de salida. "+
			"NO preguntes '¿desde dónde?' a menos que sea necesario.",
		location, suffix,
	)
}

// stripSystemMessages removes all system-role messages from a conversation history.
// Injected system context (location hints) should not be persisted — they're
// re-injected each turn from fresh data.
func stripSystemMessages(history []domain.ConversationMessage) []domain.ConversationMessage {
	if len(history) == 0 {
		return history
	}
	filtered := make([]domain.ConversationMessage, 0, len(history))
	for _, m := range history {
		if m.Role != "system" {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// prependSystemContext inserts a system context message at the beginning of
// the conversation history. The original history slice is NOT modified —
// a new slice is returned.
func prependSystemContext(history []domain.ConversationMessage, systemContent string) []domain.ConversationMessage {
	slog.Debug("prependSystemContext: injecting location hint as system message",
		slog.Int("history_len", len(history)),
		slog.String("hint_preview", systemContent[:min(len(systemContent), 150)]),
	)
	systemMsg := domain.ConversationMessage{
		Role:      "system",
		Content:   systemContent,
		Timestamp: time.Now(),
	}
	return append([]domain.ConversationMessage{systemMsg}, history...)
}

// =============================================================================
// Command builders
// =============================================================================

// buildFlightCommand builds a search_flights.Command from a TravelIntent.
// Resolved GL/HL/Currency defaults from the handler are applied here.
func buildFlightCommand(intent *domain.TravelIntent, cmd Command) search_flights.Command {
	flightCmd := search_flights.Command{
		Adults:      1,
		TripType:    "round_trip",
		TravelClass: "economy",
		SortBy:      "top",
		Stops:       "any",
		Limit:       10,
	}

	// Apply resolved defaults from the handler's ResolveSearchDefaults call
	if cmd.GL != "" {
		flightCmd.GL = &cmd.GL
	}
	if cmd.HL != "" {
		flightCmd.HL = &cmd.HL
	}
	if cmd.Currency != "" {
		flightCmd.Currency = &cmd.Currency
	}

	if intent.FlightParams != nil {
		p := intent.FlightParams
		flightCmd.Departure = p.Departure
		flightCmd.Arrival = p.Arrival
		flightCmd.OutboundDate = p.OutboundDate
		flightCmd.ReturnDate = p.ReturnDate
		if p.Adults > 0 {
			flightCmd.Adults = p.Adults
		}
		if p.TripType != "" {
			flightCmd.TripType = p.TripType
		}
		if p.TravelClass != "" {
			flightCmd.TravelClass = p.TravelClass
		}
		if p.Stops != "" {
			flightCmd.Stops = p.Stops
		}
		if p.MaxPrice != nil {
			flightCmd.MaxPrice = p.MaxPrice
		}
		if p.SortBy != "" {
			flightCmd.SortBy = p.SortBy
		}
	}

	return flightCmd
}

// buildHotelCommand builds a search_hotels.Command from a TravelIntent.
// Resolved GL/HL/Currency defaults from the handler are applied here.
func buildHotelCommand(intent *domain.TravelIntent, cmd Command) search_hotels.Command {
	hotelCmd := search_hotels.Command{
		Adults: 1,
	}

	// Apply resolved defaults from the handler's ResolveSearchDefaults call
	if cmd.GL != "" {
		hotelCmd.GL = &cmd.GL
	}
	if cmd.HL != "" {
		hotelCmd.HL = &cmd.HL
	}
	if cmd.Currency != "" {
		hotelCmd.Currency = &cmd.Currency
	}

	if intent.HotelParams != nil {
		p := intent.HotelParams
		hotelCmd.Query = p.Query
		hotelCmd.CheckInDate = p.CheckInDate
		hotelCmd.CheckOutDate = p.CheckOutDate
		if p.Adults > 0 {
			hotelCmd.Adults = p.Adults
		}
		if p.Children > 0 {
			hotelCmd.Children = p.Children
		}
		if p.Rating != nil {
			hotelCmd.Rating = p.Rating
		}
		if p.MaxPrice != nil {
			hotelCmd.MaxPrice = p.MaxPrice
		}
		hotelCmd.FreeCancellation = p.FreeCancellation
	}

	return hotelCmd
}

// =============================================================================
// Response message generators
// =============================================================================

// generateFlightsMessage generates a human-readable message for flight results.
func generateFlightsMessage(intent *domain.TravelIntent, resp *search_flights.Response) string {
	if intent.FollowUp != "" {
		return intent.FollowUp
	}
	bestCount := len(resp.BestFlights)
	otherCount := len(resp.OtherFlights)
	total := bestCount + otherCount
	if total == 0 {
		return "No encontré vuelos con esos criterios. ¿Querés ajustar algo?"
	}
	return fmt.Sprintf("Encontré %d vuelos. Acá están los resultados.", total)
}

// generateHotelsMessage generates a human-readable message for hotel results.
func generateHotelsMessage(intent *domain.TravelIntent, resp *search_hotels.Response) string {
	if intent.FollowUp != "" {
		return intent.FollowUp
	}
	count := len(resp.Properties)
	if count == 0 {
		return "No encontré hoteles con esos criterios. ¿Querés ajustar algo?"
	}
	return fmt.Sprintf("Encontré %d alojamientos. Acá están los resultados.", count)
}

// =============================================================================
// Param normalization — sanitizes AI-extracted params before SerpAPI calls
// =============================================================================

// normalizeIntent sanitizes AI-extracted FlightParams and HotelParams
// so they match the SerpAPI expected format (IATA codes, string enums).
// Called after AI returns intent but before any search execution.
func (uc *UseCase) normalizeIntent(ctx context.Context, intent *domain.TravelIntent) {
	if intent.FlightParams != nil {
		uc.normalizeFlightParams(ctx, intent.FlightParams)
	}
	if intent.HotelParams != nil {
		uc.normalizeHotelParams(ctx, intent.HotelParams)
	}
}

// normalizeFlightParams applies IATA resolution, country→IATA fallback,
// enum normalization, and date fixes.
func (uc *UseCase) normalizeFlightParams(ctx context.Context, p *domain.FlightSearchRequest) {
	// IATA resolution: AI often returns city names ("Madrid") instead of IATA codes ("MAD").
	// Also handles country names ("Perú") via country→main-airport fallback.
	if uc.iataResolver == nil {
		slog.WarnContext(ctx, "normalizeFlightParams: iataResolver is nil — AI-extracted city names will NOT be resolved to IATA codes. SerpAPI may reject invalid departure/arrival values.")
	} else {
		// Resolve departure
		if !isValidIATA(p.Departure) {
			if resolved, err := uc.iataResolver(ctx, p.Departure); err == nil && resolved != "" {
				p.Departure = resolved
			} else if iata, found := airports.ResolveCountryToIATA(p.Departure); found {
				// Country name fallback: "Perú" → "LIM"
				p.Departure = iata
			}
		}
		// Resolve arrival
		if !isValidIATA(p.Arrival) {
			if resolved, err := uc.iataResolver(ctx, p.Arrival); err == nil && resolved != "" {
				p.Arrival = resolved
			} else if iata, found := airports.ResolveCountryToIATA(p.Arrival); found {
				// Country name fallback: "Perú" → "LIM"
				p.Arrival = iata
			}
		}
	}

	// Normalize string enums: AI sometimes returns numbers or empty strings
	// instead of valid SerpAPI values.
	p.Stops = normalizeStops(p.Stops)

	if !validSortBy[p.SortBy] {
		p.SortBy = "top"
	}

	if !validTravelClass[p.TravelClass] {
		p.TravelClass = "economy"
	}

	if !validTripType[p.TripType] {
		p.TripType = "round_trip"
	}

	// Fix hallucinated dates: AI trained on 2025 data often returns 2025 dates
	// when the current year is 2026. Adjust to current year.
	p.OutboundDate = normalizeDate(p.OutboundDate)
	p.ReturnDate = normalizeDate(p.ReturnDate)
}

// normalizeHotelParams resolves AI-extracted city names to valid SerpAPI hotel queries.
//
// SerpAPI's google_hotels engine expects a city/place name (e.g., "París, Francia"),
// NOT an IATA airport code (e.g., "CDG"). This function uses accent-stripped IATA
// lookup to find the matching airport entry, then constructs the hotel query from
// the city and country fields.
//
// Fallback chain:
//  1. Strip accents from query (é→e, í→i, etc.)
//  2. Resolve to airport entry via IATA dataset (accent-stripped)
//  3. If found → build query as "{City}, {Country}" (e.g., "París, Francia")
//  4. If not found → use accent-stripped original query as-is
func (uc *UseCase) normalizeHotelParams(ctx context.Context, p *domain.HotelSearchRequest) {
	// Fix hallucinated dates: AI trained on 2025 data often returns 2025 dates
	// when the current year is 2026. Adjust to current year.
	p.CheckInDate = normalizeDate(p.CheckInDate)
	p.CheckOutDate = normalizeDate(p.CheckOutDate)

	if p.Query == "" {
		return
	}

	// Strip accents before IATA lookup: "París" → "Paris", "Múnich" → "Munich".
	stripped := airports.StripAccents(p.Query)

	// Tier 1: Try IATA resolution with accent-stripped query.
	// airports.ResolveIATA also strips accents internally, so this is belt-and-suspenders.
	entry, err := airports.ResolveIATA(ctx, nil, stripped)
	if err == nil && entry != nil {
		p.Query = formatHotelQuery(entry)
		return
	}

	// Tier 2: Try country name → main city resolution.
	// "Peru" → "Lima, Perú", "España" → "Madrid, España"
	if city, found := airports.ResolveCountryToMainCity(stripped); found {
		p.Query = city
		return
	}

	// Tier 3: IATA + country resolution failed — fallback to accent-stripped original query.
	if stripped != p.Query {
		p.Query = stripped
	}
}

// formatHotelQuery builds a SerpAPI-compatible hotel query from an airport entry.
// Format: "{City}, {Country}" — e.g., "París, Francia", "Madrid, España".
// Falls back to just City or just Country if one is missing.
func formatHotelQuery(entry *airports.AirportEntry) string {
	if entry.City != "" && entry.Country != "" {
		return entry.City + ", " + entry.Country
	}
	if entry.City != "" {
		return entry.City
	}
	return entry.Country
}

// =============================================================================
// Normalization helpers
// =============================================================================

// isValidIATA checks whether a string looks like an IATA airport code
// (exactly 3 uppercase ASCII letters).
func isValidIATA(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := range 3 {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

// normalizeStops maps AI numeric values to valid SerpAPI stops strings.
// AI returns "0" (empty), "1", "2" — SerpAPI expects "any", "nonstop", "max_1", "max_2".
func normalizeStops(s string) string {
	switch s {
	case "0", "nonstop":
		return "nonstop"
	case "1", "max_1":
		return "max_1"
	case "2", "max_2":
		return "max_2"
	default:
		return "any"
	}
}

// normalizeDate adjusts hallucinated dates where the AI returns a year
// older than the current year (trained on historical data). If the date year
// is before the current year, it's replaced with the current year.
func normalizeDate(date string) string {
	if len(date) < 4 {
		return date
	}
	yearStr := date[:4]
	var year int
	if _, err := fmt.Sscanf(yearStr, "%d", &year); err != nil || year < 2000 {
		return date
	}
	currentYear := time.Now().Year()
	if year < currentYear {
		return fmt.Sprintf("%d%s", currentYear, date[4:])
	}
	return date
}

// Valid enum sets for SerpAPI flight search parameters.
// Alineado con search_flights/command.go: top, price, departure_time,
// arrival_time, duration, emissions.
var validSortBy = map[string]bool{
	"top": true, "price": true, "departure_time": true,
	"arrival_time": true, "duration": true, "emissions": true,
}

var validTravelClass = map[string]bool{
	"economy": true, "premium_economy": true, "business": true, "first": true,
}

var validTripType = map[string]bool{
	"round_trip": true, "one_way": true,
}

// =============================================================================
// AI interpretation cache (blake3)
// =============================================================================

// interpretWithCache checks the blake3 cache for a previously interpreted
// message+history combination. Only complete intents ("flights", "hotels",
// "both") are cached. Incomplete and ambiguous queries always go to the AI
// interpreter for fresh follow-up questions.
// language is passed to the AI interpreter so the system prompt includes
// the user's detected language directive.
func (uc *UseCase) interpretWithCache(ctx context.Context, message string, history []domain.ConversationMessage, language string) (*domain.TravelIntent, bool, error) {
	// Compute blake3 hash of message + history + language for cache key (REQ-W5)
	hash := blake3Hash(message, history, language)
	cacheKey := fmt.Sprintf("ai:interpret:%x", hash)

	// Try cache first (skip if no cache configured)
	if uc.interpCache != nil {
		if cached, err := uc.interpCache.Get(ctx, cacheKey); err == nil && cached != nil {
			return cached, true, nil
		}
	}

	// Cache miss → call interpreter
	intent, err := uc.interpreter.Parse(ctx, message, history, language)
	if err != nil {
		return nil, false, err
	}

	// Only cache complete intents — incomplete/ambiguous queries
	// need fresh follow-up questions per conversation context
	if uc.interpCache != nil && isCompleteIntent(intent.Type) {
		_ = uc.interpCache.Set(ctx, cacheKey, intent, uc.interpCacheTTL)
	}

	return intent, false, nil
}

// isCompleteIntent returns true for intent types that produce search results.
// Incomplete and ambiguous intents are NOT cached because they need
// fresh follow-up questions based on conversation context.
func isCompleteIntent(intentType string) bool {
	return intentType == "flights" || intentType == "hotels" || intentType == "both"
}

// blake3Hash computes a blake3 hash of the message concatenated with serialized
// conversation history and language. The hash is deterministic: same message +
// same history + same language always produces the same key, enabling cache hits
// across conversations. Language is included so same text in different languages
// produces different cache entries (REQ-W5).
func blake3Hash(message string, history []domain.ConversationMessage, language string) []byte {
	h := blake3.New(32, nil)

	// Hash the message
	h.Write([]byte(message))

	// Hash the language tag
	h.Write([]byte(language))

	// Hash each history entry
	for _, msg := range history {
		h.Write([]byte(msg.Role))
		h.Write([]byte(msg.Content))
	}

	return h.Sum(nil)
}

// =============================================================================
// Tool call execution (Phase 2)
// =============================================================================

// ExecuteToolCalls dispatches multiple tool calls concurrently using wg.Go().
// Each tool call executes independently; partial failures are collected.
// Results array preserves the order of the input toolCalls slice.
//
// convCtx provides resolved defaults (country_code, language, currency) to prefill
// tool call arguments when the AI omits them.
func (uc *UseCase) ExecuteToolCalls(ctx context.Context, w http.ResponseWriter, toolCalls []ToolCall, convCtx ConversationContext) []ToolResult {
	results := make([]ToolResult, len(toolCalls))

	var wg sync.WaitGroup
	for i, tc := range toolCalls {
		wg.Go(func() {
			result := uc.executeSingleToolCall(ctx, w, tc, convCtx)
			results[i] = result
		})
	}
	wg.Wait()

	return results
}

// executeSingleToolCall dispatches a single tool call to the appropriate searcher.
// convCtx provides resolved defaults (country_code, language, currency) to prefill
// tool call arguments when the AI omits them.
func (uc *UseCase) executeSingleToolCall(ctx context.Context, w http.ResponseWriter, tc ToolCall, convCtx ConversationContext) ToolResult {
	result := ToolResult{
		CallID: tc.ID,
		Name:   tc.Name,
	}

	switch tc.Name {
	case "search_hotels":
		cmd, err := ParseHotelToolCall(tc.Arguments)
		if err != nil {
			result.Error = err
			result.Content = fmt.Sprintf(`{"error": "invalid arguments: %s"}`, err.Error())
			return result
		}

		// Normalize hallucinated dates (AI may return past years).
		// The model is trained on historical data and often uses last year.
		if cmd.CheckInDate != "" {
			cmd.CheckInDate = normalizeDate(cmd.CheckInDate)
		}
		if cmd.CheckOutDate != "" {
			cmd.CheckOutDate = normalizeDate(cmd.CheckOutDate)
		}

		// Prefill GL/HL/Currency from conversation context when the AI omits them.
		// DragonflyDB v1.38+: GL must be a lowercase 2-letter country code.
		if cmd.GL == nil && convCtx.CountryCode != "" {
			gl := strings.ToLower(convCtx.CountryCode)
			cmd.GL = &gl
		}
		if cmd.HL == nil && convCtx.Language != "" {
			cmd.HL = &convCtx.Language
		}
		if cmd.Currency == nil && convCtx.Currency != "" {
			cmd.Currency = &convCtx.Currency
		}

		// Default min_price=1 to filter out hotels without prices (Google Maps placeholders).
		// Matches the default in search_hotels.UseCase.Execute.
		if cmd.MinPrice == nil || *cmd.MinPrice == 0 {
			minPrice := 1.0
			cmd.MinPrice = &minPrice
		}

		// Set destination
		result.Destination = cmd.Query

		resp, err := uc.hotelSearcher.Execute(ctx, cmd)
		if err != nil {
			if !errors.Is(err, domain.ErrNoResults) {
				result.Error = err
				result.Content = fmt.Sprintf(`{"error": "%s"}`, err.Error())
				return result
			}
			result.Content = `{"properties": [], "results_state": "empty"}`
			return result
		}

		// Marshal response to JSON
		data, marshalErr := json.Marshal(resp)
		if marshalErr != nil {
			result.Error = marshalErr
			result.Content = fmt.Sprintf(`{"error": "marshal failed: %s"}`, marshalErr.Error())
			return result
		}
		result.Content = string(data)

	case "search_flights":
		cmd, err := ParseFlightToolCall(tc.Arguments)
		if err != nil {
			result.Error = err
			result.Content = fmt.Sprintf(`{"error": "invalid arguments: %s"}`, err.Error())
			return result
		}

		// Normalize hallucinated dates (AI may return past years).
		if cmd.OutboundDate != "" {
			cmd.OutboundDate = normalizeDate(cmd.OutboundDate)
		}
		if cmd.ReturnDate != "" {
			cmd.ReturnDate = normalizeDate(cmd.ReturnDate)
		}

		// Prefill GL/HL/Currency from conversation context when the AI omits them.
		// DragonflyDB v1.38+: GL must be a lowercase 2-letter country code.
		if cmd.GL == nil && convCtx.CountryCode != "" {
			gl := strings.ToLower(convCtx.CountryCode)
			cmd.GL = &gl
		}
		if cmd.HL == nil && convCtx.Language != "" {
			cmd.HL = &convCtx.Language
		}
		if cmd.Currency == nil && convCtx.Currency != "" {
			cmd.Currency = &convCtx.Currency
		}

		// Set destination
		result.Destination = fmt.Sprintf("%s→%s", cmd.Departure, cmd.Arrival)

		resp, err := uc.flightSearcher.Execute(ctx, cmd)
		if err != nil {
			if !errors.Is(err, domain.ErrNoResults) {
				result.Error = err
				result.Content = fmt.Sprintf(`{"error": "%s"}`, err.Error())
				return result
			}
			result.Content = `{"best_flights": [], "other_flights": [], "results_state": "empty"}`
			return result
		}

		data, marshalErr := json.Marshal(resp)
		if marshalErr != nil {
			result.Error = marshalErr
			result.Content = fmt.Sprintf(`{"error": "marshal failed: %s"}`, marshalErr.Error())
			return result
		}
		result.Content = string(data)

	case "emit_medical_alerts":
		alerts, parseErr := ParseMedicalAlertsToolCall(tc.Arguments)
		if parseErr != nil {
			result.Error = parseErr
			result.Content = fmt.Sprintf(`{"error": "invalid alerts: %s"}`, parseErr.Error())
			return result
		}

		domainAlerts := make([]domain.MedicalAlert, len(alerts))
		copy(domainAlerts, alerts)

		if writeErr := WriteMedicalAlertsEvent(w, domainAlerts); writeErr != nil {
			result.Error = writeErr
			result.Content = fmt.Sprintf(`{"error": "failed to write alert event: %s"}`, writeErr.Error())
			return result
		}

		result.Content = fmt.Sprintf(`{"emitted":true,"count":%d}`, len(alerts))

	case "get_destination_weather":
		cmd, parseErr := ParseDestinationWeatherToolCall(tc.Arguments)
		if parseErr != nil {
			result.Error = parseErr
			result.Content = fmt.Sprintf(`{"error": "invalid arguments: %s"}`, parseErr.Error())
			return result
		}

		if uc.dstWeatherSearcher == nil {
			result.Error = fmt.Errorf("destination weather searcher not available")
			result.Content = `{"error": "destination weather not available"}`
			return result
		}

		// Set human-readable destination from coordinates
		result.Destination = fmt.Sprintf("%.4f,%.4f", cmd.Lat, cmd.Lng)

		weather, execErr := uc.dstWeatherSearcher.Execute(ctx, cmd)
		if execErr != nil {
			result.Error = execErr
			result.Content = fmt.Sprintf(`{"error": "weather fetch failed: %s"}`, execErr.Error())
			return result
		}

		// Emit SSE weather event (weather may be nil = graceful fallback)
		if writeErr := WriteWeatherEvent(w, result.Destination, weather); writeErr != nil {
			result.Error = writeErr
			result.Content = fmt.Sprintf(`{"error": "failed to write weather event: %s"}`, writeErr.Error())
			return result
		}

		if weather != nil {
			data, marshalErr := json.Marshal(weather)
			if marshalErr != nil {
				result.Error = marshalErr
				result.Content = fmt.Sprintf(`{"error": "marshal failed: %s"}`, marshalErr.Error())
				return result
			}
			result.Content = string(data)
		} else {
			result.Content = `{"weather":null}`
		}

	default:
		result.Error = fmt.Errorf("unknown tool: %s", tc.Name)
		result.Content = fmt.Sprintf(`{"error": "unknown tool: %s"}`, tc.Name)
	}

	return result
}

// BuildToolResultMessages converts ToolResults into domain.ConversationMessage
// with role "tool" for injection back into the AI conversation.
func BuildToolResultMessages(results []ToolResult) []domain.ConversationMessage {
	messages := make([]domain.ConversationMessage, 0, len(results))
	for _, r := range results {
		content := r.Content
		if content == "" && r.Error != nil {
			content = fmt.Sprintf(`{"error": "%s"}`, r.Error.Error())
		}
		messages = append(messages, domain.ConversationMessage{
			Role:       "tool",
			Content:    content,
			ToolCallID: r.CallID,
			Timestamp:  time.Now(),
		})
	}
	return messages
}

// =============================================================================
// ExecuteChatStream — Streaming orchestration with tool calling (Phase 3)
// =============================================================================

// ExecuteChatStream orchestrates a chat-style search stream with tool calling.
// It sends messages + tools to the AI, streams text chunks, executes tool calls
// when requested, injects results back, and continues up to maxTurns rounds.
//
// conversationID is the client-provided conversation UUID. If empty, a new UUID
// is generated. This preserves the same conversation_id across multi-turn tool-calling
// sessions so GET /conversations/{id} returns the full history (REQ-C2).
//
// convCtx provides resolved defaults (country_code, language, currency) to prefill
// tool call arguments when the AI omits them. Pass an empty ConversationContext
// if no conversation-level context is available.
//
// Returns the number of tool call rounds executed.
func (uc *UseCase) ExecuteChatStream(ctx context.Context, w http.ResponseWriter, userID string, conversationID string, messages []domain.ChatMessage, tools []map[string]interface{}, maxTurns int, convCtx ConversationContext) (int, error) {
	if uc.toolCallStreamer == nil {
		return 0, fmt.Errorf("tool call streamer: %w", domain.ErrAIUnavailable)
	}
	if maxTurns <= 0 {
		maxTurns = 3
	}

	turnCount := 0
	var allResults []ToolResult // accumulate across turns for SearchCache persistence

	for turn := 0; turn < maxTurns; turn++ {
		// 1. Stream AI response token-by-token via onChunk callback.
		// Each text delta from DeepSeek is sent to the frontend immediately
		// as an SSE "chunk" event before the full response is complete.
		var accumulatedText string
		result, err := uc.toolCallStreamer.ChatWithToolsStream(ctx, messages, tools, func(text string) {
			accumulatedText += text
			WriteChunkEvent(w, text)
		})
		if err != nil {
			WriteErrorEvent(w, fmt.Sprintf("AI error: %v", err))
			return turnCount, fmt.Errorf("chat with tools: %w", err)
		}

		// 2. If no tool calls, we're done (text already streamed via onChunk)
		if len(result.ToolCalls) == 0 {
			break
		}

		// 4. Convert domain.ToolCall → ai_search.ToolCall for execution
		turnCount++
		toolCalls := make([]ToolCall, len(result.ToolCalls))
		for i, tc := range result.ToolCalls {
			toolCalls[i] = ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			}
		}

		// 5. Execute tool calls concurrently
		results := uc.ExecuteToolCalls(ctx, w, toolCalls, convCtx)
		allResults = append(allResults, results...)

		// 6. Emit search SSE events for each result
		// Build lookup: callID → AI-extracted search params with corrected dates
		searchParamsByCallID := make(map[string]map[string]interface{}, len(toolCalls))
		for _, tc := range toolCalls {
			if tc.Arguments == nil {
				continue
			}
			// Normalize hallucinated dates (AI may return wrong year)
			dateFields := []string{"outbound_date", "return_date", "check_in_date", "check_out_date"}
			for _, f := range dateFields {
				if v, ok := tc.Arguments[f].(string); ok && v != "" {
					tc.Arguments[f] = normalizeDate(v)
				}
			}
			searchParamsByCallID[tc.ID] = tc.Arguments
		}
		for _, r := range results {
			searchType := r.Name
			switch r.Name {
			case "search_hotels":
				searchType = "hotels"
			case "search_flights":
				searchType = "flights"
			}

			var data interface{}
			if r.Error == nil && r.Content != "" {
				json.Unmarshal([]byte(r.Content), &data)
			}
			WriteSearchEvent(w, r.Destination, searchType, data, searchParamsByCallID[r.CallID])
		}

		// 7. Add assistant message with tool calls to history
		assistantMsg := domain.ChatMessage{
			Role:             "assistant",
			Content:          result.AssistantText,
			ReasoningContent: result.ReasoningContent,
		}
		assistantMsg.ToolCalls = make([]domain.ToolCall, len(result.ToolCalls))
		copy(assistantMsg.ToolCalls, result.ToolCalls)
		messages = append(messages, assistantMsg)

		// 8. Add tool result messages to history
		for _, r := range results {
			content := r.Content
			if content == "" && r.Error != nil {
				content = fmt.Sprintf(`{"error": "%s"}`, r.Error.Error())
			}
			messages = append(messages, domain.ChatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: r.CallID,
			})
		}
	}

	// Emit done event with conversation ID.
	// REQ-C2: reuse client-provided conversation_id for multi-turn consistency.
	convIDStr := conversationID
	if convIDStr == "" {
		convID, _ := uuid.NewV7()
		convIDStr = convID.String()
	}

	WriteDoneEvent(w, convIDStr, turnCount)

	// Persist conversation state for retrieval via GET /conversations/{id}.
	// Convert accumulated ChatMessages to ConversationMessages for storage.
	if uc.convStore != nil {
		convMsgs := make([]domain.ConversationMessage, len(messages))
		for i, msg := range messages {
			convMsgs[i] = domain.ConversationMessage{
				Role:       msg.Role,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
				ToolCalls:  msg.ToolCalls,
				Timestamp:  time.Now(),
			}
		}
		// Build SearchCache from tool call results so the frontend
		// can restore search results when loading a previous conversation.
		searchCache := make(map[string]*CachedSearch)
		for _, r := range allResults {
			if r.Error != nil || r.Content == "" {
				continue
			}
			searchType := r.Name
			switch r.Name {
			case "search_hotels":
				searchType = "hotels"
			case "search_flights":
				searchType = "flights"
			}
			searchCache[r.CallID] = &CachedSearch{
				Response:    json.RawMessage(r.Content),
				Destination: r.Destination,
				Type:        searchType,
			}
		}
		conv := &Conversation{
			ID:          convIDStr,
			UserID:      userID,
			Messages:    convMsgs,
			SearchCache: searchCache,
			Context:     convCtx,
			TurnCount:   turnCount,
			MaxTurns:    maxTurns,
		}
		if err := uc.convStore.Save(ctx, conv); err != nil {
			slog.WarnContext(ctx, "ai_search: failed to persist streaming conversation",
				slog.String("conversation_id", convIDStr),
				slog.String("error", err.Error()),
			)
		}
	}

	return turnCount, nil
}
