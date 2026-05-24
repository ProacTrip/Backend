// Handler HTTP para GET /v1/user/profile/documents/types.
// Thin handler: público sin autenticación, delega al usecase.
package document_types

import (
	"net/http"

	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// Handler procesa GET /v1/user/profile/documents/types.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle retorna el catálogo de tipos de documento.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")

	types, err := h.usecase.Execute(c.Request().Context())
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"document_types": types,
	})
}
