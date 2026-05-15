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
// Handler — endpoint HTTP de AI search
// =============================================================================

// Handler processes AI-powered unified search HTTP requests.
type Handler struct {
	usecase     *UseCase
	rdb         *redis.Client
	defaultsCfg shared.SearchDefaultConfig
	RateLimiter *ratelimit.RateLimiter
}

// NewHandler creates a new AI search handler.
func NewHandler(usecase *UseCase, rdb *redis.Client, defaultsCfg shared.SearchDefaultConfig) *Handler {
	return &Handler{usecase: usecase, rdb: rdb, defaultsCfg: defaultsCfg}
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

		// Resolve defaults
		gl, hl, currency := shared.ResolveSearchDefaults(
			c.Request().Context(),
			h.rdb,
			shared.UserIDFromContext(c),
			c.RealIP(),
			nil, nil, nil,
			h.defaultsCfg,
		)
		cmd.GL = gl
		cmd.HL = hl
		cmd.Currency = currency
		cmd.ClientIP = c.RealIP()

		userID := shared.UserIDFromContext(c)

		// Send "thinking" event so the client knows processing has started
		sseEvent(c, "status", map[string]string{"status": "thinking"})

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

		// Rate limit provider headers
		if h.RateLimiter != nil {
			if rlResult, err := h.RateLimiter.ProviderStatus(c.Request().Context(), "serpapi"); err == nil {
				shared.SetRateLimitHeaders(c, rlResult)
			}
		}

		data, _ := json.Marshal(resp)
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

	// Validate the command before delegating to usecase.
	// This gives us explicit control over the HTTP status code
	// without relying on domain error mappers in the handler layer.
	if err := cmd.Validate(); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Resolve GL/HL/Currency from 4-tier priority chain.
	// The AI search doesn't expose explicit client params for these,
	// so Tier 1 is always skipped. Tiers 2-4 apply (profile prefs →
	// environment cache → config defaults).
	gl, hl, currency := shared.ResolveSearchDefaults(
		c.Request().Context(),
		h.rdb,
		shared.UserIDFromContext(c),
		c.RealIP(),
		nil, nil, nil, // no explicit client params in AI search
		h.defaultsCfg,
	)
	cmd.GL = gl
	cmd.HL = hl
	cmd.Currency = currency
	cmd.ClientIP = c.RealIP()

	// Extract userID from context (set by auth middleware).
	// Empty string for anonymous users.
	userID := shared.UserIDFromContext(c)

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
