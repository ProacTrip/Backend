// Caso de uso: Obtener documento (GET /v1/user/documents/:document_id).
// Verifica ownership y retorna el documento con sus metadatos.
package get_document

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// DocRepo es el puerto para obtener un documento.
type DocRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	DocRepo DocRepo
}

// UseCase implementa la obtención de documentos.
type UseCase struct {
	docRepo DocRepo
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{docRepo: deps.DocRepo}
}

// Execute obtiene el documento previa verificación de ownership.
func (uc *UseCase) Execute(ctx context.Context, documentID, userIDStr string) (*domain.UserDocument, error) {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	docID, err := uuid.Parse(documentID)
	if err != nil {
		return nil, domain.ErrDocumentNotFound
	}

	doc, err := uc.docRepo.GetByID(ctx, docID)
	if err != nil {
		return nil, err
	}

	// Verificar ownership
	if doc.UserID != userID {
		return nil, domain.ErrDocumentNotFound
	}

	return doc, nil
}

// ErrDocumentNotFound es re-exportado para uso en tests.
var ErrDocumentNotFound = domain.ErrDocumentNotFound
