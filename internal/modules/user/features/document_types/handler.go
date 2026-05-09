// Handler HTTP para GET /v1/user/documents/types.
// Retorna el catálogo de tipos de documento (seed data).
// Endpoint público sin autenticación.
package document_types

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// TypeRepo es el puerto para obtener tipos de documento.
type TypeRepo interface {
	GetTypes(ctx context.Context) ([]domain.DocumentType, error)
}

// Handler procesa GET /v1/user/documents/types.
type Handler struct {
	repo TypeRepo
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(repo TypeRepo) *Handler {
	return &Handler{repo: repo}
}

// Handle retorna el catálogo de tipos de documento.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")

	types, err := h.repo.GetTypes(c.Request().Context())
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"document_types": types,
	})
}
