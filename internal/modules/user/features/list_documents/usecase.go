// Caso de uso: Listar documentos (GET /v1/user/profile/documents).
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
// Valida status y document_type contra los enums documentados en USER_API.md.
func (uc *UseCase) Execute(ctx context.Context, userIDStr, statusFilter, docTypeFilter string) ([]*domain.UserDocument, error) {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// Validar status filter — solo los 5 estados OCR documentados
	if statusFilter != "" && !isValidOCRStatus(statusFilter) {
		return nil, domain.ErrInvalidEnum
	}

	// Validar document_type filter contra el catálogo de códigos
	if docTypeFilter != "" && !isValidDocumentType(docTypeFilter) {
		return nil, domain.ErrInvalidDocumentType
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

// isValidOCRStatus verifica que el status sea uno de los 5 estados documentados.
func isValidOCRStatus(s string) bool {
	switch domain.OCRStatus(s) {
	case domain.OCRStatusQueued, domain.OCRStatusProcessing,
		domain.OCRStatusCompleted, domain.OCRStatusRejected, domain.OCRStatusFailed:
		return true
	default:
		return false
	}
}

// isValidDocumentType verifica que el código de tipo de documento sea válido.
func isValidDocumentType(code string) bool {
	switch code {
	case "passport", "national_id", "drivers_license", "visa",
		"travel_insurance", "vaccination_cert", "boarding_pass", "receipt":
		return true
	default:
		return false
	}
}
