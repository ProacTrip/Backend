// Caso de uso: Generar URL prefirmada para descarga de documento.
// Verifica ownership, OCR status, y genera presigned GET URL de R2.
package get_document_download_url

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/storage"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Puertos Locales
// =============================================================================

// DocumentRepo es el puerto local para obtener metadata del documento.
type DocumentRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

// StorageService es el puerto local para generar URLs prefirmadas de descarga.
type StorageService interface {
	GenerateDownloadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps agrupa las dependencias del caso de uso.
type UseCaseDeps struct {
	DocRepo  DocumentRepo
	Storage  StorageService
}

// UseCase orquesta la generación de URL prefirmada para descarga.
type UseCase struct {
	docRepo DocumentRepo
	storage StorageService
}

// NewUseCase crea un nuevo caso de uso para generar URLs de descarga.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		docRepo: deps.DocRepo,
		storage: deps.Storage,
	}
}

// Execute verifica ownership, estado OCR y genera URL prefirmada de R2.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*DownloadURLResponse, error) {
	if err := cmd.Validate(); err != nil {
		return nil, fmt.Errorf("validar comando: %w", err)
	}

	// 1. Obtener metadata del documento
	doc, err := uc.docRepo.GetByID(ctx, cmd.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("obtener documento: %w", err)
	}

	// 2. Verificar ownership
	if doc.UserID != cmd.UserID {
		return nil, domain.ErrDocumentNotFound
	}

	// 3. Solo disponible cuando completed o rejected
	if doc.OCRStatus != domain.OCRStatusCompleted && doc.OCRStatus != domain.OCRStatusRejected {
		return nil, domain.ErrDocumentNotReady
	}

	// 4. Generar URL prefirmada GET (TTL: 15 minutos)
	expiry := 15 * time.Minute
	expiresAt := time.Now().Add(expiry)

	downloadURL, err := uc.storage.GenerateDownloadURL(ctx, storage.SecureBucket(), doc.StorageKey, expiry)
	if err != nil {
		return nil, fmt.Errorf("generar download URL: %w", err)
	}

	return &DownloadURLResponse{
		DownloadURL: downloadURL,
		ExpiresAt:   expiresAt.UTC().Format("2006-01-02T15:04:05Z"),
		FileName:    doc.FileName,
	}, nil
}
