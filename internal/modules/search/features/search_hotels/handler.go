// Handler HTTP para búsqueda de hoteles y vacation rentals.
// Expuesto en POST /v1/search/hotels.
package search_hotels

import (
	"log/slog"
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler — endpoint HTTP de search hotels
// =============================================================================

// Handler processes hotel search HTTP requests.
type Handler struct {
	usecase *UseCase
}

// NewHandler creates a new search hotels handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle processes the hotel search request.
// Route: POST /v1/search/hotels
func (h *Handler) Handle(c *echo.Context) error {
	var cmd Command

	// Set defaults before binding so they act as fallbacks
	cmd.Adults = 1
	cmd.Currency = "USD"

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	slog.Error("DEBUG: handler called, body bound",
		slog.String("query", cmd.Query),
		slog.String("check_in", cmd.CheckInDate),
		slog.String("check_out", cmd.CheckOutDate),
		slog.Int("adults", cmd.Adults),
	)

	// Quick validation before passing to use case
	if cmd.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	if cmd.CheckInDate == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "check_in_date is required")
	}
	if cmd.CheckOutDate == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "check_out_date is required")
	}

	slog.Error("DEBUG: validation passed, calling usecase")

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "search_hotels failed",
			slog.String("error", err.Error()),
			slog.String("query", cmd.Query),
		)
		return httperr.MapError(c, err)
	}

	slog.Error("DEBUG: usecase returned response",
		slog.Int("property_count", len(resp.Properties)),
		slog.Bool("from_cache", resp.FromCache),
		slog.String("results_state", resp.ResultsState),
	)

	c.Response().Header().Set("Cache-Control", "public, max-age=300, s-maxage=300, stale-while-revalidate=300")
	return c.JSON(http.StatusOK, resp)
}
