// Handler HTTP para detalles de vuelo.
// expuesta en POST /api/v1/flights/details.
package flight_details

import (
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler — endpoint HTTP de flight details
// =============================================================================

// Handler processes flight details HTTP requests.
type Handler struct {
	usecase     *UseCase
	rdb         *redis.Client
	defaultsCfg shared.SearchDefaultConfig
}

// NewHandler creates a new flight details handler.
func NewHandler(usecase *UseCase, rdb *redis.Client, defaultsCfg shared.SearchDefaultConfig) *Handler {
	return &Handler{usecase: usecase, rdb: rdb, defaultsCfg: defaultsCfg}
}

// Handle processes the flight details request.
// Route: POST /api/v1/flights/details
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	// Set defaults before binding so they act as fallbacks
	cmd.Adults = 1

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	// Resolve GL/HL/Currency from the 4-tier priority chain
	gl, hl, currency := shared.ResolveSearchDefaults(
		c.Request().Context(),
		h.rdb,
		shared.UserIDFromContext(c), // userID from auth middleware, "" for anonymous
		c.RealIP(),
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
		slog.ErrorContext(c.Request().Context(), "flight_details failed",
			slog.String("error", err.Error()),
		)
		return httperr.MapError(c, err)
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=900, s-maxage=900, stale-while-revalidate=300")
	return c.JSON(http.StatusOK, resp)
}
