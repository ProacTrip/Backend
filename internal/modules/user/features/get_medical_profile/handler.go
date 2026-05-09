// Handler HTTP para GET /v1/user/profile/medical.
// Extrae user_claims del contexto (cookie auth) y retorna el perfil médico desencriptado.
package get_medical_profile

import (
	"errors"
	"net/http"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler procesa GET /v1/user/profile/medical.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea un nuevo handler para el perfil médico.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae el user_id de los claims del token y retorna el perfil médico.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	cmd := Command{UserID: claims.UserID.String()}

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
