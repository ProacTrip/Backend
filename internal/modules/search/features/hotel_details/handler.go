// Handler HTTP para detalles de hotel.
// Expuesto en POST /v1/search/hotel-details.
package hotel_details

import (
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler — endpoint HTTP de hotel details
// =============================================================================

// Handler processes hotel details HTTP requests.
type Handler struct {
	usecase     *UseCase
	rdb         *redis.Client
	defaultsCfg shared.SearchDefaultConfig
}

// NewHandler creates a new hotel details handler.
func NewHandler(usecase *UseCase, rdb *redis.Client, defaultsCfg shared.SearchDefaultConfig) *Handler {
	return &Handler{usecase: usecase, rdb: rdb, defaultsCfg: defaultsCfg}
}

// Handle processes the hotel details request.
// Route: POST /v1/search/hotel-details
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	// Set defaults before binding so they act as fallbacks
	cmd.Adults = 2

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	// Quick validation before passing to use case
	if cmd.ID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id (property_token) is required")
	}
	if cmd.CheckInDate == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "check_in_date is required")
	}
	if cmd.CheckOutDate == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "check_out_date is required")
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
		slog.ErrorContext(c.Request().Context(), "hotel_details failed",
			slog.String("error", err.Error()),
			slog.String("id", cmd.ID),
		)
		return httperr.MapError(c, err)
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=900, s-maxage=900, stale-while-revalidate=300")
	return c.JSON(http.StatusOK, resp)
}
