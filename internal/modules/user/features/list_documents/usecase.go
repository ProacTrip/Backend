// Caso de uso: Listar documentos (GET /v1/user/documents).
// Valida filtros y consulta el repositorio.
package list_documents

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// DocRepo es el puerto para listar documentos.
type DocRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error)
	GetByUserIDFiltered(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	DocRepo DocRepo
}

// UseCase implementa el listado de documentos.
type UseCase struct {
	docRepo DocRepo
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{docRepo: deps.DocRepo}
}

// Execute lista los documentos del usuario con filtros opcionales.
func (uc *UseCase) Execute(ctx context.Context, userIDStr, statusFilter, docTypeFilter string) ([]*domain.UserDocument, error) {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	var docs []*domain.UserDocument
	if statusFilter != "" || docTypeFilter != "" {
		docs, err = uc.docRepo.GetByUserIDFiltered(ctx, userID, domain.OCRStatus(statusFilter), docTypeFilter)
	} else {
		docs, err = uc.docRepo.GetByUserID(ctx, userID)
	}
	if err != nil {
		return nil, err
	}

	if docs == nil {
		docs = []*domain.UserDocument{}
	}

	return docs, nil
}
