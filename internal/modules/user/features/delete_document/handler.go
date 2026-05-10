// Handler HTTP para DELETE /v1/user/documents/:document_id.
// Thin handler: extrae claims y document_id, delega al usecase.
package delete_document

import (
	"net/http"

	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// Handler procesa DELETE /v1/user/documents/:document_id.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae claims y document_id del path, y delega al usecase.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	documentID := c.Param("document_id")

	if err := h.usecase.Execute(c.Request().Context(), documentID, claims.UserID.String()); err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Documento y todos los archivos asociados eliminados correctamente.",
	})
}
