// Handler HTTP para AI search.
// Expuesto en POST /v1/search/ai. Soporta streaming via "stream": true en el body.
package ai_search

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

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
	RateLimiter *ratelimit.RateLimiter
}

// NewHandler creates a new AI search handler.
// userProfile may be nil for anonymous-only deployments or tests.
func NewHandler(usecase *UseCase, convStore ConversationStore, rdb *redis.Client, defaultsCfg shared.SearchDefaultConfig, userProfile domain.UserProfilePort) *Handler {
	return &Handler{usecase: usecase, convStore: convStore, rdb: rdb, defaultsCfg: defaultsCfg, userProfile: userProfile}
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
func sseError(c *echo.Context, message string) {
	sseEventRaw(c, "error", fmt.Sprintf(`{"error":"%s"}`, message))
}

// =============================================================================
// Tool Calling streaming dispatch (Phase 2)
// =============================================================================

// handleToolCallingStream dispatches the request to ExecuteChatStream for
// tool-calling-aware AI streaming. It builds the initial chat messages with
// location context, constructs the tool definitions, and delegates SSE
// streaming to the usecase.
func (h *Handler) handleToolCallingStream(c *echo.Context, cmd Command, userID string) {
	ctx := c.Request().Context()

	// Resolve location hint for system context injection
	hint := h.usecase.resolveLocationHint(ctx, userID, cmd.ClientIP)

	// Build initial chat messages
	messages := make([]domain.ChatMessage, 0, 2)
	if hint != "" {
		messages = append(messages, domain.ChatMessage{
			Role:    "system",
			Content: hint,
		})
	}
	messages = append(messages, domain.ChatMessage{
		Role:    "user",
		Content: cmd.Message,
	})

	// Build tool definitions from typed ToolDefs
	tools := buildDefaultTools()

	// Build conversation context from resolved defaults.
	// CountryCode comes from config defaults (DEFAULT_COUNTRY_CODE env).
	convCtx := ConversationContext{
		CountryCode: h.defaultsCfg.CountryCode,
		Language:    cmd.HL,
		Currency:    cmd.Currency,
	}

	maxTurns := h.usecase.maxTurnsForUser(userID)

	_, err := h.usecase.ExecuteChatStream(ctx, c.Response(), messages, tools, maxTurns, convCtx)
	if err != nil {
		// Error already sent as SSE event by ExecuteChatStream.
		slog.ErrorContext(ctx, "ai_search: ExecuteChatStream failed",
			slog.String("error", err.Error()),
			slog.String("message", cmd.Message),
		)
	}
}

// buildDefaultTools converts the typed ToolDefs for search_hotels and
// search_flights into the []map[string]interface{} format expected by the
// ToolCallStreamer interface.
func buildDefaultTools() []map[string]interface{} {
	hotelJSON, _ := json.Marshal(SearchHotelsToolDef())
	flightJSON, _ := json.Marshal(SearchFlightsToolDef())

	var hotelMap, flightMap map[string]interface{}
	json.Unmarshal(hotelJSON, &hotelMap)
	json.Unmarshal(flightJSON, &flightMap)

	return []map[string]interface{}{hotelMap, flightMap}
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
