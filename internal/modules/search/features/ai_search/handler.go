// Handler HTTP para AI search.
// Expuesto en POST /v1/search/ai. Soporta streaming via "stream": true en el body.
package ai_search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

// =============================================================================
// User ID resolution — auth claims or anonymous cookie
// =============================================================================

// resolveUserID extracts the user identifier from the request context.
// For authenticated users, it returns the user ID from the auth middleware.
// For anonymous users, it returns the anon_token cookie value as the identifier.
func resolveUserID(c *echo.Context) string {
	if userID := shared.UserIDFromContext(c); userID != "" {
		return userID
	}
	return ratelimit.AnonIDFromContext(c)
}

// =============================================================================
// Handler — endpoint HTTP de AI search
// =============================================================================

// Handler processes AI-powered unified search HTTP requests.
type Handler struct {
	usecase     *UseCase
	convStore   ConversationStore // new ConversationStore for CRUD endpoints
	rdb         *redis.Client
	defaultsCfg shared.SearchDefaultConfig
	userProfile domain.UserProfilePort
	userHealth  domain.UserHealthPort // medical/travel/document context
	RateLimiter *ratelimit.RateLimiter
}

// NewHandler creates a new AI search handler.
// userProfile may be nil for anonymous-only deployments or tests.
// userHealth may be nil for deployments without medical context.
func NewHandler(usecase *UseCase, convStore ConversationStore, rdb *redis.Client, defaultsCfg shared.SearchDefaultConfig, userProfile domain.UserProfilePort, userHealth domain.UserHealthPort) *Handler {
	return &Handler{usecase: usecase, convStore: convStore, rdb: rdb, defaultsCfg: defaultsCfg, userProfile: userProfile, userHealth: userHealth}
}

// Handle processes the AI search request.
// Route: POST /v1/search/ai
//
// When cmd.Stream is true, the response is delivered via SSE:
//   - "status": "thinking" event sent immediately
//   - "result" event with the full JSON response on completion
//   - "error" event if anything goes wrong
//
// When h.usecase is nil (AI interpreter not configured at bootstrap),
// returns 503 Service Unavailable per RFC 9457 (or SSE error event if streaming).
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	if err := c.Bind(&cmd); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Stream path — SSE headers must be set before any response body
	if cmd.Stream {
		c.Response().Header().Set("Content-Type", "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")
		c.Response().WriteHeader(http.StatusOK)

		// Nil usecase → AI not configured — send error event over SSE
		if h.usecase == nil {
			sseError(c, "AI search no disponible — el servicio de IA no está configurado en este entorno")
			return nil
		}

		if err := cmd.Validate(); err != nil {
			sseError(c, err.Error())
			return nil
		}

		// Resolve user prefs + env data + fallback defaults
		h.resolveContext(c, &cmd)

		userID := resolveUserID(c)

		// Send "thinking" event so the client knows processing has started
		sseEvent(c, "status", map[string]string{"status": "thinking"})

		// Dispatch to Tool Calling streaming path if available.
		// Falls back to legacy Execute() for non-tool-calling deployments.
		if h.usecase.toolCallStreamer != nil {
			h.handleToolCallingStream(c, cmd, userID)
			return nil
		}

		resp, err := h.usecase.Execute(c.Request().Context(), cmd, userID)
		if err != nil {
			slog.ErrorContext(c.Request().Context(), "ai_search stream failed",
				slog.String("error", err.Error()),
				slog.String("message", cmd.Message),
			)
			if errors.Is(err, domain.ErrRateLimitExceeded) {
				c.Response().Header().Set("Retry-After", "60")
			}
			sseError(c, err.Error())
			return nil
		}

		resp.FromCache = false

		// Exact search: send the full JSON response as a single "result" event
		// Rate limit provider headers
		if h.RateLimiter != nil {
			if rlResult, err := h.RateLimiter.ProviderStatus(c.Request().Context(), "serpapi"); err == nil {
				shared.SetRateLimitHeaders(c, rlResult)
			}
		}

		data, err := json.Marshal(resp)
		if err != nil {
			slog.ErrorContext(c.Request().Context(), "ai_search: json marshal failed",
				slog.String("error", err.Error()),
			)
			sseError(c, "internal error")
			return nil
		}
		sseEventRaw(c, "result", string(data))
		return nil
	}

	// Non-stream path — existing behavior
	// Nil usecase → AI not configured (503, not 404)
	if h.usecase == nil {
		return c.JSON(http.StatusServiceUnavailable, serrors.ErrServiceUnavailable(
			"AI search no disponible — el servicio de IA no está configurado en este entorno",
			nil,
		).WithInstance(c.Request().URL.Path))
	}

	// Validation is handled by the usecase (uc.Execute calls cmd.Validate()).
	// Resolve user prefs + env data + fallback defaults
	h.resolveContext(c, &cmd)

	// Extract userID from context (set by auth middleware or anon cookie).
	// Empty string for anonymous users with no cookie.
	userID := resolveUserID(c)

	resp, err := h.usecase.Execute(c.Request().Context(), cmd, userID)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "ai_search failed",
			slog.String("error", err.Error()),
			slog.String("message", cmd.Message),
		)
		if errors.Is(err, domain.ErrRateLimitExceeded) {
			shared.SetRateLimitExceededHeaders(c, h.RateLimiter, "serpapi")
		}
		return httperr.MapError(c, err)
	}

	resp.FromCache = false

	// Rate limit provider headers (SerpAPI quota)
	if h.RateLimiter != nil {
		if rlResult, err := h.RateLimiter.ProviderStatus(c.Request().Context(), "serpapi"); err == nil {
			shared.SetRateLimitHeaders(c, rlResult)
		}
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, resp)
}

// =============================================================================
// Context resolution — user prefs, env cache, fallback defaults
// =============================================================================

// resolveContext populates cmd with resolved user preferences (currency, language)
// from the UserProfilePort, falling back to config defaults.
//
// Resolution order:
//
//	Currency / HL:  user profile prefs (auth users) → config defaults
//
// Location (lat/lng/timezone/country_code) is NO LONGER resolved here.
// The backend resolves it from c.RealIP() → env:{ip} cache inside the usecase.
func (h *Handler) resolveContext(c *echo.Context, cmd *Command) {
	ctx := c.Request().Context()
	userID := shared.UserIDFromContext(c)
	clientIP := c.RealIP()

	// Resolve currency/language from user profile port (auth users only)
	if userID != "" && h.userProfile != nil {
		if curr, lang, err := h.userProfile.GetPreferences(ctx, userID); err == nil {
			if cmd.Currency == "" {
				cmd.Currency = curr
			}
			if cmd.HL == "" {
				cmd.HL = lang
			}
		}
	}

	// Fallback to config defaults for currency/language
	if cmd.Currency == "" {
		cmd.Currency = h.defaultsCfg.Currency
	}
	if cmd.HL == "" {
		cmd.HL = h.defaultsCfg.Language
	}

	cmd.ClientIP = clientIP
}

// sseEvent sends a named SSE event with JSON payload.
func sseEvent(c *echo.Context, event string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		slog.Error("sseEvent: marshal failed", "error", err)
		return
	}
	sseEventRaw(c, event, string(payload))
}

// sseEventRaw sends a named SSE event with a raw string payload.
func sseEventRaw(c *echo.Context, event, data string) {
	fmt.Fprintf(c.Response(), "event: %s\ndata: %s\n\n", event, data)
	if flusher, ok := c.Response().(http.Flusher); ok {
		flusher.Flush()
	}
}

// sseError sends an error SSE event to the client.
// Uses sseEvent (which calls json.Marshal) to ensure valid, parseable JSON
// even when the error message contains special characters like double-quotes
// or backslashes. Fixes REQ-C1: SSE Error JSON Safety.
func sseError(c *echo.Context, message string) {
	sseEvent(c, "error", map[string]string{"error": message})
}

// isOffTopicMessage detects clearly non-travel questions so the backend
// can redirect immediately without spending AI credits.
func isOffTopicMessage(msg string) bool {
	lower := strings.ToLower(msg)
	patterns := []string{
		"mundial", "fifa", "world cup",
		"capital de", "presidente",
		"receta", "cocinar",
		"código", "programar", "javascript", "python", "java",
		"clima hoy", "qué hora es", "cuántos días tiene",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// =============================================================================
// Tool Calling streaming dispatch (Phase 2)
// =============================================================================

// handleToolCallingStream dispatches the request to ExecuteChatStream for
// tool-calling-aware AI streaming. It builds the initial chat messages with
// location context and medical context (if authenticated), constructs the tool
// definitions, and delegates SSE streaming to the usecase.
func (h *Handler) handleToolCallingStream(c *echo.Context, cmd Command, userID string) {
	ctx := c.Request().Context()

	// Off-topic guard: if the user asks a clearly non-travel question,
	// return a redirect message via SSE without calling the AI.
	if isOffTopicMessage(cmd.Message) {
		sseEvent(c, "chunk", map[string]string{"content": "Soy un asistente de viajes. ¿Querés que busque vuelos u hoteles para vos?"})
		sseEventRaw(c, "done", fmt.Sprintf(`{"conversation_id":"","turn_count":0}`))
		return
	}

	// Resolve location hint for system context injection
	hint := h.usecase.resolveLocationHint(ctx, userID, cmd.ClientIP)

	// Load previous conversation history for multi-turn context.
	var prevMessages []domain.ChatMessage
	if cmd.ConversationID != "" {
		prevConv, loadErr := h.convStore.Load(ctx, cmd.ConversationID)
		if loadErr != nil {
			slog.WarnContext(ctx, "handleToolCallingStream: failed to load conversation",
				slog.String("conversation_id", cmd.ConversationID),
				slog.String("error", loadErr.Error()),
			)
		}
		if prevConv != nil {
			for _, m := range prevConv.Messages {
				prevMessages = append(prevMessages, domain.ChatMessage{
					Role:       m.Role,
					Content:    m.Content,
					ToolCallID: m.ToolCallID,
					ToolCalls:  m.ToolCalls,
				})
			}
			slog.DebugContext(ctx, "handleToolCallingStream: loaded previous conversation",
				slog.String("conversation_id", cmd.ConversationID),
				slog.Int("prev_messages", len(prevMessages)),
			)
		}
	}

	// System prompt for tool-calling path.
	messages := []domain.ChatMessage{
		{
			Role: "system",
			Content: "Sos un asistente de viajes que busca vuelos y hoteles usando herramientas. " +
				"CRÍTICO: Los resultados de búsqueda (vuelos, hoteles, precios) se muestran AUTOMÁTICAMENTE en un panel separado. " +
				"NO repitas los resultados en tu texto. Tu respuesta debe ser un RESUMEN BREVE (1-2 frases). " +
				"NUNCA escribas tablas, listas de precios, ni detalles de vuelos/hoteles en el chat. " +
				"Ejemplo de respuesta CORRECTA: 'Encontré 5 hoteles en Madrid. El mejor valorado es X. ¿Filtro por algo más?' " +
				"Ejemplo de respuesta INCORRECTA: '| Hotel | Precio |...' — NUNCA hagas esto. " +
				"NO uses emojis. NO menciones que estás revisando fechas o corrigiendo años. " +
				"Recordá TODO lo que el usuario te dijo en mensajes anteriores (destino, fechas, presupuesto, origen). " +
				"Si el usuario ya te dijo su ciudad de origen, NO preguntes '¿desde dónde?' de nuevo. " +
				"Si ya te dijo el destino, NO preguntes '¿a dónde?' de nuevo. " +
				"Si ya te dijo 'todo el mes' o '31 días', NO preguntes cuántos días.",
		},
	}

	// Inject medical/travel/document context FIRST, so the location hint
	// (injected after) takes priority. This prevents the AI from using
	// the user's nationality as their current location when a GeoIP-based
	// location hint is available.
	includeMedicalAlerts := false
	_, accessErr := c.Cookie("__Secure-access_token")
	_, legacyErr := c.Cookie("access_token")
	hasAuthCookie := accessErr == nil || legacyErr == nil
	if hasAuthCookie && h.userHealth != nil {
		if medicalMsg := h.buildMedicalContextMessage(ctx, userID); medicalMsg != "" {
			messages = append(messages, domain.ChatMessage{
				Role:    "system",
				Content: medicalMsg,
			})
		}
		includeMedicalAlerts = true
	}

	// Location hint injected AFTER medical context so it overrides any
	// nationality-based location assumptions. Based on /v1/environment (GeoIP).
	if hint != "" {
		messages = append(messages, domain.ChatMessage{
			Role:    "system",
			Content: hint,
		})
	}

	// Style rules — these are part of the AI's system prompt in the adapter,
	// NOT injected as separate system messages that could leak into the UI.

	// Append previous conversation history so the AI remembers what was
	// discussed in earlier turns. System messages are re-injected fresh above.
	// Tool messages are required by DeepSeek's API contract (every assistant
	// message with tool_calls MUST be followed by tool responses), but we
	// replace the raw search result JSON with a compact summary to stay under
	// token limits — the full JSON can be 10KB+ per message.
	for _, m := range prevMessages {
		if m.Role == "system" {
			continue
		}
		if m.Role == "tool" {
			// Send a minimal acknowledgement so DeepSeek doesn't reject the request.
			// The AI already has the search results from the assistant's tool_calls.
			messages = append(messages, domain.ChatMessage{
				Role:       "tool",
				Content:    `{"status":"completed"}`,
				ToolCallID: m.ToolCallID,
			})
			continue
		}
		messages = append(messages, m)
	}

	messages = append(messages, domain.ChatMessage{
		Role:    "user",
		Content: cmd.Message,
	})

	// Build tool definitions from typed ToolDefs
	tools := buildDefaultTools(includeMedicalAlerts)

	// Build conversation context from resolved defaults.
	// CountryCode comes from config defaults (DEFAULT_COUNTRY_CODE env).
	convCtx := ConversationContext{
		CountryCode: h.defaultsCfg.CountryCode,
		Language:    cmd.HL,
		Currency:    cmd.Currency,
	}

	maxTurns := h.usecase.maxTurnsForUser(userID)

	_, err := h.usecase.ExecuteChatStream(ctx, c.Response(), userID, cmd.ConversationID, messages, tools, maxTurns, convCtx)
	if err != nil {
		// Error already sent as SSE event by ExecuteChatStream.
		slog.ErrorContext(ctx, "ai_search: ExecuteChatStream failed",
			slog.String("error", err.Error()),
			slog.String("message", cmd.Message),
		)
	}
}

// buildMedicalContextMessage fetches medical profile, travel preferences,
// document context, and nationality, then formats them as a Spanish system
// message for the AI. Returns empty string if all contexts are empty.
func (h *Handler) buildMedicalContextMessage(ctx context.Context, userID string) string {
	var parts []string

	// Medical profile
	medical, err := h.userHealth.GetMedicalContext(ctx, userID)
	if err == nil && medical != nil {
		medParts := []string{}
		if len(medical.Allergies) > 0 {
			medParts = append(medParts, "Alergias: ["+strings.Join(medical.Allergies, ", ")+"]")
		}
		if len(medical.Conditions) > 0 {
			medParts = append(medParts, "Condiciones: ["+strings.Join(medical.Conditions, ", ")+"]")
		}
		if len(medical.Medications) > 0 {
			medParts = append(medParts, "Medicamentos: ["+strings.Join(medical.Medications, ", ")+"]")
		}
		if len(medical.Vaccinations) > 0 {
			medParts = append(medParts, "Vacunas: ["+strings.Join(medical.Vaccinations, ", ")+"]")
		}
		if medical.BloodType != "" {
			medParts = append(medParts, "Tipo de sangre: "+medical.BloodType)
		}
		if len(medParts) > 0 {
			parts = append(parts, "PERFIL MÉDICO: "+strings.Join(medParts, ". ")+".")
		}
	}

	// Travel preferences
	travel, err := h.userHealth.GetTravelPreferences(ctx, userID)
	if err == nil && travel != nil {
		travelParts := []string{}
		if travel.PreferredClass != "" {
			travelParts = append(travelParts, "Clase "+travel.PreferredClass)
		}
		if travel.SeatPreference != "" {
			travelParts = append(travelParts, travel.SeatPreference)
		}
		if travel.MealPreference != "" {
			travelParts = append(travelParts, travel.MealPreference)
		}
		if travel.AvoidLayovers {
			layoverMsg := "evitar escalas"
			if travel.MaxLayoverDuration > 0 {
				layoverMsg += fmt.Sprintf(" (máx %dmin)", travel.MaxLayoverDuration)
			}
			travelParts = append(travelParts, layoverMsg)
		}
		if len(travelParts) > 0 {
			parts = append(parts, "PREFERENCIAS DE VIAJE: "+strings.Join(travelParts, ", ")+".")
		}
	}

	// Document context
	docs, err := h.userHealth.GetDocumentContext(ctx, userID)
	if err == nil {
		if len(docs) == 0 {
			parts = append(parts, "DOCUMENTOS: Sin documentos de viaje registrados.")
		} else {
			docStrs := make([]string, 0, len(docs))
			for _, doc := range docs {
				var docStr string
				switch doc.Type {
				case "passport":
					docStr = "Pasaporte"
				case "visa":
					docStr = "Visa"
				default:
					docStr = doc.Type
				}
				if doc.IssuingCountry != "" {
					docStr += " " + doc.IssuingCountry
				}
				if doc.Number != "" {
					docStr += " #" + doc.Number
				}
				if doc.ValidUntil != "" {
					docStr += " (vence " + doc.ValidUntil + ")"
				}
				docStrs = append(docStrs, docStr)
			}
			parts = append(parts, "DOCUMENTOS: "+strings.Join(docStrs, ". ")+".")
		}
	}

	// Nationality (IMPORTANT: this is the user's nationality/passport country,
	// NOT their current location — do NOT confuse the two).
	nationality := h.userHealth.GetNationality(ctx, userID)
	if nationality != "" {
		parts = append(parts, "NACIONALIDAD: "+nationality+" (esto es tu nacionalidad, NO tu ubicación actual).")
	}

	if len(parts) == 0 {
		return ""
	}

	// Add the anti-repeat rule
	parts = append(parts, "IMPORTANTE: Cuando detectes riesgos médicos o de viaje, usa la herramienta emit_medical_alerts UNA sola vez con TODAS las alertas. NO menciones estas alertas en tu texto de respuesta — el usuario las ve en una ventana emergente separada.")

	return strings.Join(parts, "\n")
}

// buildDefaultTools converts the typed ToolDefs for search_hotels,
// search_flights, and optionally emit_medical_alerts into the
// []map[string]interface{} format expected by the ToolCallStreamer interface.
func buildDefaultTools(includeMedicalAlerts bool) []map[string]interface{} {
	hotelJSON, _ := json.Marshal(SearchHotelsToolDef())
	flightJSON, _ := json.Marshal(SearchFlightsToolDef())
	weatherJSON, _ := json.Marshal(GetDestinationWeatherToolDef())

	var hotelMap, flightMap, weatherMap map[string]interface{}
	json.Unmarshal(hotelJSON, &hotelMap)
	json.Unmarshal(flightJSON, &flightMap)
	json.Unmarshal(weatherJSON, &weatherMap)

	tools := []map[string]interface{}{hotelMap, flightMap, weatherMap}

	if includeMedicalAlerts {
		alertJSON, _ := json.Marshal(EmitMedicalAlertsToolDef())
		var alertMap map[string]interface{}
		json.Unmarshal(alertJSON, &alertMap)
		tools = append(tools, alertMap)
	}

	return tools
}

// =============================================================================
// Conversation CRUD handlers (Phase 5)
// =============================================================================

// HandleListConversations returns the user's active conversations.
// Route: GET /v1/search/ai/conversations
func (h *Handler) HandleListConversations(c *echo.Context) error {
	userID := resolveUserID(c)
	if userID == "" {
		// No auth and no anon cookie — conversations tracked by
		// conversation_id in the frontend only.
		return c.JSON(http.StatusOK, []ConversationPreview{})
	}

	previews, err := h.convStore.ListUserConversations(c.Request().Context(), userID)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "HandleListConversations: failed",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		return httperr.MapError(c, err)
	}

	if previews == nil {
		previews = []ConversationPreview{}
	}

	return c.JSON(http.StatusOK, previews)
}

// HandleGetConversation returns the full state of a conversation.
// Route: GET /v1/search/ai/conversations/{id}
//
// This is the F5 recovery endpoint (REQ-009): the frontend can reconstruct
// the entire chat UI from the returned state.
func (h *Handler) HandleGetConversation(c *echo.Context) error {
	convID := c.Param("id")
	if convID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "conversation ID is required")
	}

	conv, err := h.convStore.Load(c.Request().Context(), convID)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "HandleGetConversation: Load failed",
			slog.String("conversation_id", convID),
			slog.String("error", err.Error()),
		)
		return httperr.MapError(c, err)
	}

	if conv == nil {
		return echo.NewHTTPError(http.StatusNotFound, "conversation not found or expired")
	}

	// Ownership check: only the conversation owner can access it.
	// Anonymous users are identified by their anon_token cookie value.
	userID := resolveUserID(c)
	if conv.UserID != "" && conv.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "conversation belongs to another user")
	}

	return c.JSON(http.StatusOK, conv)
}

// HandleDeleteConversation removes a conversation.
// Route: DELETE /v1/search/ai/conversations/{id}
func (h *Handler) HandleDeleteConversation(c *echo.Context) error {
	convID := c.Param("id")
	if convID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "conversation ID is required")
	}

	userID := resolveUserID(c)

	// Load first to check ownership (unless anonymous).
	conv, err := h.convStore.Load(c.Request().Context(), convID)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "HandleDeleteConversation: Load failed",
			slog.String("conversation_id", convID),
			slog.String("error", err.Error()),
		)
		return httperr.MapError(c, err)
	}

	if conv == nil {
		return echo.NewHTTPError(http.StatusNotFound, "conversation not found")
	}

	// Ownership check
	if conv.UserID != "" && conv.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "conversation belongs to another user")
	}

	if err := h.convStore.Delete(c.Request().Context(), convID, conv.UserID); err != nil {
		slog.ErrorContext(c.Request().Context(), "HandleDeleteConversation: Delete failed",
			slog.String("conversation_id", convID),
			slog.String("error", err.Error()),
		)
		return httperr.MapError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
