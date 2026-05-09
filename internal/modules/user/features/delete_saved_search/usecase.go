// Caso de uso: Eliminar búsqueda guardada (DELETE /v1/user/saved-searches/:search_id).
package delete_saved_search

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

type SavedSearchRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// =============================================================================
// UseCase
// =============================================================================

type UseCaseDeps struct {
	SavedSearchRepo SavedSearchRepo
}

type UseCase struct {
	repo SavedSearchRepo
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{repo: deps.SavedSearchRepo}
}

// Execute elimina una búsqueda guardada previa verificación de ownership.
func (uc *UseCase) Execute(ctx context.Context, userIDString, searchIDString string) error {
	userID, err := uuid.Parse(userIDString)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	searchID, err := uuid.Parse(searchIDString)
	if err != nil {
		return fmt.Errorf("invalid search_id: %w", err)
	}

	// Verificar ownership
	existing, err := uc.repo.GetByID(ctx, searchID)
	if err != nil {
		if errors.Is(err, domain.ErrSearchNotFound) {
			return err
		}
		return fmt.Errorf("get saved search: %w", err)
	}
	if existing.UserID != userID {
		return domain.ErrSearchNotFound
	}

	if err := uc.repo.Delete(ctx, searchID); err != nil {
		return fmt.Errorf("delete saved search: %w", err)
	}

	return nil
}
