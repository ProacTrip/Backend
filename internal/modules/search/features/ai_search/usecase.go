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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"lukechampine.com/blake3"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
	searchshared "github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	"github.com/ProacTrip/Backend/internal/modules/search/shared/airports"
	envdomain "github.com/ProacTrip/Backend/internal/modules/environment/domain"
	sharedEnv "github.com/ProacTrip/Backend/internal/shared/environment"
	userprefs "github.com/ProacTrip/Backend/internal/modules/user/features/shared"
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

// ConversationStore abstracts conversation persistence.
type ConversationStore interface {
	GetConversation(ctx context.Context, id string) (*domain.ConversationState, error)
	SaveConversation(ctx context.Context, conv *domain.ConversationState) error
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
	interpreter    domain.AIInterpreter
	flightSearcher FlightSearcher
	hotelSearcher  HotelSearcher
	convStore      ConversationStore
	interpCache    InterpretationCache
	iataResolver   func(ctx context.Context, query string) (string, error)
	rdb            *redis.Client                   // Dragonfly for location hint resolution (env:{ip}, profile:{userID}:prefs)
	defaultsCfg    searchshared.SearchDefaultConfig // Fallback defaults (DEFAULT_COUNTRY_CODE, etc.)
	anonMaxTurns int
	authMaxTurns int
}

// UseCaseDeps bundles dependencies for the AI search use case.
type UseCaseDeps struct {
	AIInterpreter  domain.AIInterpreter
	FlightSearcher FlightSearcher
	HotelSearcher  HotelSearcher
	ConvStore      ConversationStore
	InterpCache    InterpretationCache // nil = no caching (MVP mode)
	AnonMaxTurns   int
	AuthMaxTurns   int

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
}

// NewUseCase creates a new AI search use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	if deps.AnonMaxTurns <= 0 {
		deps.AnonMaxTurns = 5
	}
	if deps.AuthMaxTurns <= 0 {
		deps.AuthMaxTurns = 10
	}
	return &UseCase{
		interpreter:    deps.AIInterpreter,
		flightSearcher: deps.FlightSearcher,
		hotelSearcher:  deps.HotelSearcher,
		convStore:      deps.ConvStore,
		interpCache:    deps.InterpCache,
		iataResolver:   deps.IATAResolver,
		rdb:            deps.RDB,
		defaultsCfg:    deps.DefaultsCfg,
		anonMaxTurns:   deps.AnonMaxTurns,
		authMaxTurns:   deps.AuthMaxTurns,
	}
}

// =============================================================================
// Execute — main orchestration
// =============================================================================

// Execute orchestrates the AI interpretation and search execution.
// userID is empty for anonymous users.
func (uc *UseCase) Execute(ctx context.Context, cmd Command, userID string) (*Response, error) {
	slog.DebugContext(ctx, "ai_search.Execute: start",
		slog.String("message", cmd.Message[:min(len(cmd.Message), 80)]),
		slog.String("conversation_id", cmd.ConversationID),
		slog.String("user_id", userID),
	)

	// 1. Validate command
	if err := cmd.Validate(); err != nil {
		slog.WarnContext(ctx, "ai_search.Execute: command validation failed",
			slog.String("error", err.Error()),
		)
		return nil, err
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
		// Wrap with ErrSearchFailed so error mapper returns 502 Bad Gateway
		// instead of 500 Internal Server Error.
		if flightErr != nil && hotelErr != nil {
			return nil, fmt.Errorf("%w: flights: %w | hotels: %w",
				domain.ErrSearchFailed, flightErr, hotelErr)
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
	var resultsJSON json.RawMessage
	if flightResp != nil || hotelResp != nil {
		combined := struct {
			Flights *search_flights.Response `json:"flights,omitzero"`
			Hotels  *search_hotels.Response  `json:"hotels,omitzero"`
		}{
			Flights: flightResp,
			Hotels:  hotelResp,
		}
		resultsJSON, _ = json.Marshal(combined)
		conv.Results = resultsJSON
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

// =============================================================================
// Location hint — injects detected user location as AI context
// =============================================================================

// The env:{ip} cache contract is defined in shared/environment/dto.go.
// We use sharedEnv.LocationDTO to decode location data from the cache.

// resolveLocationHint determines the user's location for AI context injection.
//
// Resolution strategy:
//   - Auth users: read country_code from profile:{userID}:prefs Dragonfly hash,
//     resolve country name via CountryMetadata, and try to get main airport/city.
//   - Anonymous: parse the env:{ip} Dragonfly cache (full EnvironmentResponse JSON)
//     to extract city, country, and country_code.
//   - If both fail or no data is available, returns "" (no hint injected).
//
// The returned string is a Spanish system context message instructing the AI
// to use the detected location as default departure for flight searches.
func (uc *UseCase) resolveLocationHint(ctx context.Context, userID, clientIP string) string {
	if uc.rdb == nil {
		slog.DebugContext(ctx, "resolveLocationHint: no rdb configured, skipping")
		return ""
	}

	var city, country, countryCode, iata string

	if userID != "" {
		// Authenticated user: get country_code from profile prefs.
		_, _, cc, _, found, err := userprefs.GetProfilePrefs(ctx, uc.rdb, userID)
		if err != nil || !found || cc == "" {
			slog.DebugContext(ctx, "resolveLocationHint: auth user has no profile prefs, skipping",
				slog.String("user_id", userID),
				slog.Bool("found", found),
			)
			return ""
		}
		countryCode = cc

		// Resolve country name and main airport IATA.
		if info, ok := envdomain.GetCountryInfo(cc); ok {
			country = info.Country
		} else {
			slog.DebugContext(ctx, "resolveLocationHint: unknown country code",
				slog.String("country_code", cc),
			)
			return ""
		}

		// Try to get main airport IATA for this country.
		if mainIATA, found := airports.ResolveCountryToIATA(country); found {
			iata = mainIATA
			// Try to extract the city from the airport dataset.
			if entry, err := airports.ResolveIATA(ctx, nil, mainIATA); err == nil && entry != nil {
				city = entry.City
			}
		}
	} else if clientIP != "" {
		// Anonymous user: parse env:{ip} Dragonfly cache.
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
			// Try to resolve IATA code for the city (anonymous users have city-level data).
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

	// Fallback: use DEFAULT_COUNTRY_CODE from config when no location data is available
	// (e.g. first request in local dev where /v1/environment hasn't been called).
	if city == "" && country == "" && countryCode == "" && uc.defaultsCfg.CountryCode != "" {
		cc := uc.defaultsCfg.CountryCode
		if info, ok := envdomain.GetCountryInfo(cc); ok {
			countryCode = cc
			country = info.Country
			if mainIATA, found := airports.ResolveCountryToIATA(country); found {
				iata = mainIATA
				// Try to get the main city for this country (without the ", Country" suffix).
				if cityWithCountry, found := airports.ResolveCountryToMainCity(country); found {
					// Strip ", Country" suffix if present (e.g. "Buenos Aires, Argentina" → "Buenos Aires").
					parts := strings.SplitN(cityWithCountry, ",", 2)
					city = strings.TrimSpace(parts[0])
				}
			}
			slog.DebugContext(ctx, "resolveLocationHint: resolved via default country code",
				slog.String("country_code", cc),
				slog.String("country", country),
				slog.String("city", city),
				slog.String("iata", iata),
			)
		}
	}

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
var validSortBy = map[string]bool{
	"top": true, "best": true, "fastest": true, "cheapest": true,
}

var validTravelClass = map[string]bool{
	"economy": true, "premium_economy": true, "business": true, "first": true,
}

var validTripType = map[string]bool{
	"round_trip": true, "one_way": true, "multi_city": true,
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
	// Compute blake3 hash of message + history for cache key
	hash := blake3Hash(message, history)
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
		_ = uc.interpCache.Set(ctx, cacheKey, intent, 10*time.Minute)
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
// conversation history. The hash is deterministic: same message + same history
// always produces the same key, enabling cache hits across conversations.
func blake3Hash(message string, history []domain.ConversationMessage) []byte {
	h := blake3.New(32, nil)

	// Hash the message
	h.Write([]byte(message))

	// Hash each history entry
	for _, msg := range history {
		h.Write([]byte(msg.Role))
		h.Write([]byte(msg.Content))
	}

	return h.Sum(nil)
}
