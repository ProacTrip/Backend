// Caso de uso: Obtener catálogo de tipos de documento (GET /v1/user/documents/types).
// Simple wrapper que establece el patrón feature-slice.
package document_types

import (
	"context"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// TypeRepo es el puerto para obtener tipos de documento.
type TypeRepo interface {
	GetTypes(ctx context.Context) ([]domain.DocumentType, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	TypeRepo TypeRepo
}

// UseCase implementa la obtención del catálogo de tipos de documento.
type UseCase struct {
	typeRepo TypeRepo
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{typeRepo: deps.TypeRepo}
}

// Execute retorna el catálogo estático de tipos de documento.
func (uc *UseCase) Execute(ctx context.Context) ([]domain.DocumentType, error) {
	return uc.typeRepo.GetTypes(ctx)
}
