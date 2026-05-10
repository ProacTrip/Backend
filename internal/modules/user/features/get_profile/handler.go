// Handler HTTP para GET /v1/user/profile.
// Extrae user_claims del contexto (inyectado por auth middleware via cookie).
package get_profile

import (
	"net/http"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler procesa GET /v1/user/profile.
type Handler struct {
	usecase *UseCase
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae el user_id de los claims del token (cookie auth) y retorna el perfil.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	// Extraer user claims del contexto (inyectados por auth middleware)
	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	cmd := Command{UserID: claims.UserID.String()}

	if err := cmd.Validate(); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
