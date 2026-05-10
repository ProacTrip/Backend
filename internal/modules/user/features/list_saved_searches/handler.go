// Handler HTTP para GET /v1/user/saved-searches.
package list_saved_searches

import (
	"net/http"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler procesa GET /v1/user/saved-searches.
type Handler struct {
	usecase *UseCase
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle lista las búsquedas guardadas del usuario autenticado.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), claims.UserID.String())
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
