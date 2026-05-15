// Handler HTTP para búsqueda de vuelos.
// Expuesto en POST /v1/search/flights.
package search_flights

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler — endpoint HTTP de search flights
// =============================================================================

// Handler processes flight search HTTP requests.
type Handler struct {
	usecase     *UseCase
	rdb         *redis.Client
	defaultsCfg shared.SearchDefaultConfig
	RateLimiter *ratelimit.RateLimiter
}

// NewHandler creates a new search flights handler.
func NewHandler(usecase *UseCase, rdb *redis.Client, defaultsCfg shared.SearchDefaultConfig) *Handler {
	return &Handler{usecase: usecase, rdb: rdb, defaultsCfg: defaultsCfg}
}

// Handle processes the flight search request.
// Route: POST /v1/search/flights
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	// Set default adults — other defaults are handled by Command.Validate()
	cmd.Adults = 1

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	// Resolve GL/HL/Currency from the 4-tier priority chain
	gl, hl, currency := shared.ResolveSearchDefaults(
		c.Request().Context(),
		h.rdb,
		shared.UserIDFromContext(c), // userID from auth middleware, "" for anonymous
		c.RealIP(),                  // clientIP for env:{ip} cache lookup
		cmd.GL, cmd.HL, cmd.Currency,
		h.defaultsCfg,
	)
	if cmd.GL == nil {
		cmd.GL = new(gl)
	}
	if cmd.HL == nil {
		cmd.HL = new(hl)
	}
	if cmd.Currency == nil {
		cmd.Currency = new(currency)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "search_flights failed",
			slog.String("error", err.Error()),
			slog.String("trip_type", cmd.TripType),
			slog.String("departure", cmd.Departure),
		)
		if errors.Is(err, domain.ErrRateLimitExceeded) {
			shared.SetRateLimitExceededHeaders(c, h.RateLimiter, "serpapi")
		}
		return httperr.MapError(c, err)
	}

	resp.FromCache = false
	resp.CachedAt = nil

	// Rate limit provider headers (SerpAPI quota)
	if h.RateLimiter != nil {
		if rlResult, err := h.RateLimiter.ProviderStatus(c.Request().Context(), "serpapi"); err == nil {
			shared.SetRateLimitHeaders(c, rlResult)
		}
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=300, s-maxage=300, stale-while-revalidate=300")
	return c.JSON(http.StatusOK, resp)
}
