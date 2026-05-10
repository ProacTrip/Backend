// Handler HTTP para DELETE /v1/user/favorites/:favorite_id.
package delete_favorite

import (
	"net/http"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler procesa DELETE /v1/user/favorites/:favorite_id.
type Handler struct {
	usecase *UseCase
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae claims y favorite_id del path, verifica ownership y elimina.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	favoriteID := c.Param("favorite_id")

	if err := h.usecase.Execute(c.Request().Context(), claims.UserID.String(), favoriteID); err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Favorito eliminado correctamente.",
	})
}
