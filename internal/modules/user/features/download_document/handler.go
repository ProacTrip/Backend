// Handler HTTP para GET /v1/user/documents/:document_id/download.
// Streamea el archivo procesado desde R2 al cliente.
// Solo disponible cuando ocr_status es completed o rejected.
package download_document

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// DownloadDocRepo es el puerto para obtener metadata del documento.
type DownloadDocRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

// DownloadR2Client es el puerto para descargar archivos de R2.
type DownloadR2Client interface {
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

// Handler procesa GET /v1/user/documents/:document_id/download.
type Handler struct {
	docRepo DownloadDocRepo
	r2      DownloadR2Client
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(docRepo DownloadDocRepo, r2 DownloadR2Client) *Handler {
	return &Handler{docRepo: docRepo, r2: r2}
}

// Handle streamea el archivo desde R2.
func (h *Handler) Handle(c *echo.Context) error {
	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	docID, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	doc, err := h.docRepo.GetByID(c.Request().Context(), docID)
	if err != nil {
		return httperr.MapError(c, err)
	}

	// Verificar ownership
	if doc.UserID != claims.UserID {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	// Solo disponible cuando completed o rejected
	if doc.OCRStatus != domain.OCRStatusCompleted && doc.OCRStatus != domain.OCRStatusRejected {
		return httperr.MapError(c, domain.ErrDocumentNotReady)
	}

	// Descargar de R2
	reader, err := h.r2.Download(c.Request().Context(), "proactrip-secure", doc.StorageKey)
	if err != nil {
		return httperr.MapError(c, fmt.Errorf("descargar documento de R2: %w", err))
	}
	defer reader.Close()

	// Setear headers de respuesta
	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, doc.FileName))

	mime := "application/octet-stream"
	if doc.MimeType != nil && *doc.MimeType != "" {
		mime = *doc.MimeType
	}
	c.Response().Header().Set("Content-Type", mime)

	c.Response().Header().Set("Cache-Control", "private, max-age=300")

	// Streamear el archivo
	if _, err := io.Copy(c.Response(), reader); err != nil {
		return fmt.Errorf("stream document: %w", err)
	}

	return nil
}
