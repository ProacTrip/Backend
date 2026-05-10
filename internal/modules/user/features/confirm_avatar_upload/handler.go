// Handler HTTP para POST /v1/user/profile/avatar/confirm.
// Verifica que el archivo existe en R2 y dispara la validación asíncrona.
package confirm_avatar_upload

import (
	"net/http"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler procesa POST /v1/user/profile/avatar/confirm.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae user_claims, bindea, valida y dispara validación asíncrona.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	// Extraer user claims del contexto (cookie auth)
	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	var cmd Command
	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	// El UserID siempre viene del token, nunca del request body
	cmd.UserID = claims.UserID.String()

	if err := cmd.Validate(); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusAccepted, resp)
}
