// Caso de uso: Descargar documento procesado.
// Verifica ownership, OCR status, y recupera el archivo de R2.
package download_document

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Puertos Locales
// =============================================================================

// DocumentRepo es el puerto local para obtener metadata del documento.
type DocumentRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

// StorageService es el puerto local para descargar archivos de R2.
type StorageService interface {
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

// =============================================================================
// Response
// =============================================================================

// Response contiene el reader del archivo y su metadata para el handler.
type Response struct {
	Reader   io.ReadCloser
	FileName string
	MimeType string
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps agrupa las dependencias del caso de uso.
type UseCaseDeps struct {
	DocRepo  DocumentRepo
	Storage  StorageService
}

// UseCase orquesta la descarga de un documento procesado.
type UseCase struct {
	docRepo DocumentRepo
	storage StorageService
}

// NewUseCase crea un nuevo caso de uso para descargar documentos.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		docRepo: deps.DocRepo,
		storage: deps.Storage,
	}
}

// Execute verifica ownership, estado OCR y descarga el archivo de R2.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
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

	// 4. Descargar de R2
	reader, err := uc.storage.Download(ctx, "proactrip-secure", doc.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("descargar documento de R2: %w", err)
	}

	// 5. Determinar MIME type
	mime := "application/octet-stream"
	if doc.MimeType != nil && *doc.MimeType != "" {
		mime = *doc.MimeType
	}

	return &Response{
		Reader:   reader,
		FileName: doc.FileName,
		MimeType: mime,
	}, nil
}
