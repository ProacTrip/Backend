// Handler HTTP para búsqueda de vuelos.
// Expuesto en POST /v1/search/flights.
package search_flights

import (
	"log/slog"
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler — endpoint HTTP de search flights
// =============================================================================

// Handler processes flight search HTTP requests.
type Handler struct {
	usecase *UseCase
}

// NewHandler creates a new search flights handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle processes the flight search request.
// Route: POST /v1/search/flights
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	// Set defaults before binding so they act as fallbacks
	cmd.Adults = 1
	cmd.TravelClass = "economy"
	cmd.Currency = "USD"
	cmd.SortBy = "top"
	cmd.Stops = "any"

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "search_flights failed",
			slog.String("error", err.Error()),
			slog.String("trip_type", cmd.TripType),
			slog.String("departure", cmd.Departure),
		)
		return httperr.MapError(c, err)
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=900, s-maxage=900, stale-while-revalidate=300")
	return c.JSON(http.StatusOK, resp)
}
