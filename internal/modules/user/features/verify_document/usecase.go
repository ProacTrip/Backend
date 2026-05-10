// Caso de uso: Verificar documento (PUT /v1/user/documents/:document_id/verify).
// ADMIN ONLY — la verificación de rol se maneja a nivel de middleware de ruta.
// Marca un documento como verificado o revierte la verificación.
// Si es un pasaporte y se marca como verificado, dispara reprocesamiento OCR.
package verify_document

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// VerifyDocRepo es el puerto para obtener y actualizar un documento.
type VerifyDocRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	Update(ctx context.Context, doc *domain.UserDocument) error
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	DocRepo   VerifyDocRepo
	Dragonfly *redis.Client
}

// UseCase implementa la verificación de documentos.
type UseCase struct {
	docRepo   VerifyDocRepo
	dragonfly *redis.Client
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		docRepo:   deps.DocRepo,
		dragonfly: deps.Dragonfly,
	}
}

// VerifyResponse es la respuesta del caso de uso.
type VerifyResponse struct {
	Message    string `json:"message"`
	IsVerified bool   `json:"is_verified"`
}

// Execute marca el documento como verificado o no verificado.
func (uc *UseCase) Execute(ctx context.Context, cmd VerifyCommand) (*VerifyResponse, error) {
	// 1. Parsear document_id
	docID, err := uuid.Parse(cmd.DocumentID)
	if err != nil {
		return nil, domain.ErrDocumentNotFound
	}

	// 2. Cargar documento
	doc, err := uc.docRepo.GetByID(ctx, docID)
	if err != nil {
		return nil, err
	}

	// 3. Marcar verificación
	if cmd.IsVerified {
		verifiedBy, parseErr := uuid.Parse(cmd.VerifiedBy)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid verified_by user_id: %w", parseErr)
		}
		doc.MarkVerified(verifiedBy)
	} else {
		doc.IsVerified = false
		doc.VerifiedBy = nil
		doc.VerifiedAt = nil
		doc.UpdatedAt = time.Now()
	}

	// 4. Guardar en repositorio
	if err := uc.docRepo.Update(ctx, doc); err != nil {
		return nil, fmt.Errorf("actualizar documento: %w", err)
	}

	// 5. Si es un pasaporte y se marca como verificado, disparar reprocesamiento OCR
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

		if uc.dragonfly != nil {
			uc.dragonfly.XAdd(ctx, &redis.XAddArgs{
				Stream: "{events}:doc:ocr",
				ID:     "*",
				Values: ocrPayload,
			})
		}
	}

	return &VerifyResponse{
		Message:    "Documento verificado correctamente.",
		IsVerified: doc.IsVerified,
	}, nil
}
