// Handler HTTP para DELETE /v1/user/saved-searches/:search_id.
package delete_saved_search

import (
	"net/http"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler procesa DELETE /v1/user/saved-searches/:search_id.
type Handler struct {
	usecase *UseCase
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae claims y search_id del path, verifica ownership y elimina.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	searchID := c.Param("search_id")

	if err := h.usecase.Execute(c.Request().Context(), claims.UserID.String(), searchID); err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Búsqueda guardada eliminada correctamente.",
	})
}
