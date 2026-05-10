// Handler HTTP para GET /v1/user/documents.
// Thin handler: extrae claims y query params, delega al usecase.
package list_documents

import (
	"net/http"

	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// Handler procesa GET /v1/user/documents.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae claims y query params, y delega al usecase.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	statusFilter := c.QueryParam("status")
	docTypeFilter := c.QueryParam("document_type")

	docs, err := h.usecase.Execute(c.Request().Context(), claims.UserID.String(), statusFilter, docTypeFilter)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"documents": docs,
	})
}
