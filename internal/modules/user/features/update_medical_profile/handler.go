// Handler HTTP para PUT /v1/user/profile/medical.
// Bindea JSON, encripta campos sensibles y actualiza el perfil médico.
package update_medical_profile

import (
	"errors"
	"net/http"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler procesa PUT /v1/user/profile/medical.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea un nuevo handler para actualizar el perfil médico.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle bindea el JSON, sobreescribe UserID con los claims y actualiza.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	// Extraer user claims del contexto (cookie auth)
	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
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
		if errors.Is(err, domain.ErrMedicalProfileNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "perfil médico no encontrado",
			})
		}
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
