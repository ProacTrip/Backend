// Caso de uso: Eliminar documento (DELETE /v1/user/documents/:document_id).
// Verifica ownership, elimina archivos de R2 y el registro de la DB.
package delete_document

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/storage"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// DocRepo es el puerto para operaciones de documento en PostgreSQL.
type DocRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// R2Client es el puerto para eliminar archivos de R2.
type R2Client interface {
	Delete(ctx context.Context, bucket, key string) error
	ListObjects(ctx context.Context, bucket, prefix string) ([]string, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	DocRepo   DocRepo
	R2        R2Client
	Dragonfly *redis.Client
}

// UseCase implementa la eliminación de documentos.
type UseCase struct {
	docRepo   DocRepo
	r2        R2Client
	dragonfly *redis.Client
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{docRepo: deps.DocRepo, r2: deps.R2, dragonfly: deps.Dragonfly}
}

// Execute elimina el documento previa verificación de ownership.
func (uc *UseCase) Execute(ctx context.Context, documentID, userIDStr string) error {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	docID, err := uuid.Parse(documentID)
	if err != nil {
		return domain.ErrDocumentNotFound
	}

	// Cargar documento
	doc, err := uc.docRepo.GetByID(ctx, docID)
	if err != nil {
		return err
	}

	// Verificar ownership
	if doc.UserID != userID {
		return domain.ErrDocumentNotFound
	}

	// Listar y eliminar archivos de R2 por prefijo
	prefixes := []string{
		fmt.Sprintf("raw/%s/%s/", doc.UserID, docID),
		fmt.Sprintf("processed/%s/%s/", doc.UserID, docID),
		fmt.Sprintf("results/%s/%s/", doc.UserID, docID),
	}

	for _, prefix := range prefixes {
		keys, listErr := uc.r2.ListObjects(ctx, storage.SecureBucket(), prefix)
		if listErr != nil {
			continue
		}
		for _, key := range keys {
			_ = uc.r2.Delete(ctx, storage.SecureBucket(), key)
		}
	}

	// Eliminar de PostgreSQL
	if err := uc.docRepo.Delete(ctx, docID); err != nil {
		return fmt.Errorf("eliminar documento de DB: %w", err)
	}

	// Limpiar keys de dedup en Dragonfly para permitir re-subida
	if uc.dragonfly != nil && doc.ContentHash != "" {
		userDedupKey := fmt.Sprintf("{dedup}:user:%s:%s", doc.UserID, doc.ContentHash)
		globalDedupKey := fmt.Sprintf("{dedup}:global:%s", doc.ContentHash)
		uc.dragonfly.Del(ctx, userDedupKey)
		uc.dragonfly.Del(ctx, globalDedupKey)
	}

	return nil
}

// ErrDocumentNotFound es re-exportado para uso en tests.
var ErrDocumentNotFound = domain.ErrDocumentNotFound
