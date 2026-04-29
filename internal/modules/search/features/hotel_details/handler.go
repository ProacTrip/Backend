// Handler HTTP para detalles de hotel.
// Expuesto en POST /v1/search/hotel-details.
package hotel_details

import (
	"log/slog"
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler — endpoint HTTP de hotel details
// =============================================================================

// Handler processes hotel details HTTP requests.
type Handler struct {
	usecase *UseCase
}

// NewHandler creates a new hotel details handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle processes the hotel details request.
// Route: POST /v1/search/hotel-details
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	// Set defaults before binding so they act as fallbacks
	cmd.Adults = 1
	cmd.Currency = "USD"

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
