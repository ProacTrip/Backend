// Handler HTTP para PUT /v1/user/documents/:document_id/verify.
// ADMIN ONLY. Verificación manual de autenticidad de documentos.
package verify_document

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// VerifyDocRepo es el puerto para obtener y actualizar un documento.
type VerifyDocRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	Update(ctx context.Context, doc *domain.UserDocument) error
}

// VerifyCommand es el body del request.
type VerifyCommand struct {
	IsVerified bool `json:"is_verified"`
}

// Handler procesa PUT /v1/user/documents/:document_id/verify.
type Handler struct {
	docRepo   VerifyDocRepo
	dragonfly *redis.Client
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(docRepo VerifyDocRepo, dragonfly *redis.Client) *Handler {
	return &Handler{docRepo: docRepo, dragonfly: dragonfly}
}

// Handle marca el documento como verificado/no verificado.
// La verificación de rol admin se maneja a nivel de middleware de ruta.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	docID, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}
	verifiedBy := claims.UserID

	var cmd VerifyCommand
	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	doc, err := h.docRepo.GetByID(c.Request().Context(), docID)
	if err != nil {
		return httperr.MapError(c, err)
	}

	// Marcar verificación
	doc.MarkVerified(verifiedBy)
	doc.IsVerified = cmd.IsVerified
	if !cmd.IsVerified {
		doc.IsVerified = false
		doc.VerifiedBy = nil
		doc.VerifiedAt = nil
	}

	if err := h.docRepo.Update(c.Request().Context(), doc); err != nil {
		return httperr.MapError(c, err)
	}

	// Si es un pasaporte que no fue auto-verificado y ahora se marca como verificado,
	// disparar reprocesamiento OCR
	if cmd.IsVerified && doc.DocumentType != nil && *doc.DocumentType == "passport" {
		ocrPayload := map[string]interface{}{
			"document_id":        docID.String(),
			"user_id":            doc.UserID.String(),
			"storage_key":        doc.StorageKey,
			"file_name":          doc.FileName,
			"detected_mime_type": "",
			"timestamp":          fmt.Sprintf("%d", time.Now().UnixMilli()),
		}
		if doc.MimeType != nil {
			ocrPayload["detected_mime_type"] = *doc.MimeType
		}

		h.dragonfly.XAdd(c.Request().Context(), &redis.XAddArgs{
			Stream: "{events}:doc:ocr",
			ID:     "*",
			Values: ocrPayload,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":     "Document verification updated.",
		"is_verified": doc.IsVerified,
	})
}
