// Handler HTTP para PUT /v1/user/documents/:document_id/verify.
// ADMIN ONLY. Verificación manual de autenticidad de documentos.
package verify_document

import (
	"net/http"

	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// Handler procesa PUT /v1/user/documents/:document_id/verify.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae claims y document_id del path, bindea el comando y delega al usecase.
// La verificación de rol admin se maneja a nivel de middleware de ruta.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	var cmd VerifyCommand
	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	cmd.DocumentID = c.Param("document_id")
	cmd.VerifiedBy = claims.UserID.String()

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
