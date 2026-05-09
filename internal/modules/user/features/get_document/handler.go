// Handler HTTP para GET /v1/user/documents/:document_id.
// Retorna los metadatos completos de un documento.
package get_document

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// GetDocRepo es el puerto para obtener un documento.
type GetDocRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

// Handler procesa GET /v1/user/documents/:document_id.
type Handler struct {
	repo GetDocRepo
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(repo GetDocRepo) *Handler {
	return &Handler{repo: repo}
}

// Handle retorna el documento con sus metadatos.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	docID, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	doc, err := h.repo.GetByID(c.Request().Context(), docID)
	if err != nil {
		return httperr.MapError(c, err)
	}

	// Verificar ownership
	if doc.UserID != claims.UserID {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	return c.JSON(http.StatusOK, doc)
}
