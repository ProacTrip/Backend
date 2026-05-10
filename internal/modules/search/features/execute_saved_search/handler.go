// Handler HTTP para POST /v1/search/execute_saved.
// Ejecuta una búsqueda guardada previamente por el usuario autenticado.
package execute_saved_search

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler processes execute saved search HTTP requests.
type Handler struct {
	usecase *UseCase
}

// NewHandler creates a new execute saved search handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle processes POST /v1/search/execute_saved.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")

	// Extraer user claims del contexto (cookie auth)
	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	var body struct {
		SavedSearchID string `json:"saved_search_id"`
	}
	if err := c.Bind(&body); err != nil {
		return httperr.MapError(c, err)
	}

	searchID, err := uuid.Parse(body.SavedSearchID)
	if err != nil {
		return httperr.MapError(c, err)
	}

	cmd := Command{
		SavedSearchID: searchID,
		UserID:        claims.UserID,
	}

	if err := cmd.Validate(); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "execute_saved_search failed",
			slog.String("error", err.Error()),
			slog.String("saved_search_id", cmd.SavedSearchID.String()),
		)
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
