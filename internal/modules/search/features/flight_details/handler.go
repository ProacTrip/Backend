// Handler HTTP para detalles de vuelo.
// expuesta en POST /api/v1/flights/details.
package flight_details

import (
	"log/slog"
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler — endpoint HTTP de flight details
// =============================================================================

// Handler processes flight details HTTP requests.
type Handler struct {
	usecase *UseCase
}

// NewHandler creates a new flight details handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle processes the flight details request.
// Route: POST /api/v1/flights/details
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	// Set defaults before binding so they act as fallbacks
	cmd.Adults = 1
	cmd.Currency = "USD"

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
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
