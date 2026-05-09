// Handler HTTP para DELETE /v1/user/documents/:document_id.
// Elimina el registro del documento y todos los archivos asociados en R2.
package delete_document

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// DeleteDocRepo es el puerto para eliminar un documento de PostgreSQL.
type DeleteDocRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// DeleteR2Client es el puerto para eliminar archivos de R2.
type DeleteR2Client interface {
	Delete(ctx context.Context, bucket, key string) error
	ListObjects(ctx context.Context, bucket, prefix string) ([]string, error)
}

// Handler procesa DELETE /v1/user/documents/:document_id.
type Handler struct {
	docRepo DeleteDocRepo
	r2      DeleteR2Client
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(docRepo DeleteDocRepo, r2 DeleteR2Client) *Handler {
	return &Handler{docRepo: docRepo, r2: r2}
}

// Handle elimina el documento y sus archivos en R2.
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

	// Cargar documento
	doc, err := h.docRepo.GetByID(c.Request().Context(), docID)
	if err != nil {
		return httperr.MapError(c, err)
	}

	// Verificar ownership
	if doc.UserID != claims.UserID {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	// Listar y eliminar archivos de R2 por prefijo
	prefixes := []string{
		fmt.Sprintf("raw/%s/%s/", doc.UserID, docID),
		fmt.Sprintf("processed/%s/%s/", doc.UserID, docID),
		fmt.Sprintf("results/%s/%s/", doc.UserID, docID),
	}

	for _, prefix := range prefixes {
		keys, err := h.r2.ListObjects(c.Request().Context(), "proactrip-secure", prefix)
		if err != nil {
			continue
		}
		for _, key := range keys {
			_ = h.r2.Delete(c.Request().Context(), "proactrip-secure", key)
		}
	}

	// Eliminar de PostgreSQL
	if err := h.docRepo.Delete(c.Request().Context(), docID); err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Document and all associated files deleted successfully.",
	})
}
