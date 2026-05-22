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

	"github.com/redis/go-redis/v9"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	sharedEnv "github.com/ProacTrip/Backend/internal/shared/environment"
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
	userProfile domain.UserProfilePort
	RateLimiter *ratelimit.RateLimiter
}

// NewHandler creates a new AI search handler.
// userProfile may be nil for anonymous-only deployments or tests.
func NewHandler(usecase *UseCase, rdb *redis.Client, defaultsCfg shared.SearchDefaultConfig, userProfile domain.UserProfilePort) *Handler {
	return &Handler{usecase: usecase, rdb: rdb, defaultsCfg: defaultsCfg, userProfile: userProfile}
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

		userID := shared.UserIDFromContext(c)
		isDiscovery := cmd.SearchModeHint == "discovery"

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

		// Discovery mode: stream the AI response as word-chunked SSE events
		if isDiscovery && resp.Mode == "discovery" && resp.Message != "" {
			if err := streamDiscoveryResponse(c.Response(), resp.Message); err != nil {
				slog.ErrorContext(c.Request().Context(), "ai_search: SSE stream write failed",
					slog.String("error", err.Error()),
				)
			}
			return nil
		}

		// Exact search: send the full JSON response as a single "result" event
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

	// Resolve user prefs + env data + fallback defaults
	h.resolveContext(c, &cmd)

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

// =============================================================================
// Context resolution — user prefs, env cache, fallback defaults
// =============================================================================

// resolveContext populates cmd with resolved user preferences (currency, language)
// from the UserProfilePort, environmental data (lat, lng, timezone, country_code)
// from the env:{ip} cache when not provided by the client, and falls back to
// config defaults for currency/language.
//
// Resolution order per field:
//
//	Currency / HL:  user profile prefs (auth users) → config defaults
//	Lat / Lng:       client-provided (omitzero=0) → env:{ip} cache
//	Timezone:         client-provided → env:{ip} cache
//	CountryCode:      client-provided → env:{ip} cache
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

	// Resolve environment data from env:{ip} cache when client didn't provide it
	needEnv := cmd.Lat == 0 && cmd.Lng == 0 || cmd.Timezone == "" || cmd.CountryCode == ""
	if needEnv && h.rdb != nil && clientIP != "" {
		if entry := h.resolveEnvCacheEntry(ctx, clientIP); entry != nil {
			if cmd.Lat == 0 && cmd.Lng == 0 {
				cmd.Lat = entry.Location.Latitude
				cmd.Lng = entry.Location.Longitude
			}
			if cmd.Timezone == "" {
				cmd.Timezone = entry.Location.Timezone
			}
			if cmd.CountryCode == "" {
				cmd.CountryCode = entry.Location.CountryCode
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

// resolveEnvCacheEntry reads the full env:{ip} cache entry from DragonflyDB.
// Returns nil if the cache is unavailable or cannot be parsed.
func (h *Handler) resolveEnvCacheEntry(ctx context.Context, ip string) *sharedEnv.CacheEntry {
	if h.rdb == nil {
		return nil
	}
	key := sharedEnv.CacheKey(ip)
	raw, err := h.rdb.Get(ctx, key).Result()
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
