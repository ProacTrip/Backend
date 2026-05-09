// Caso de uso: Eliminar favorito (DELETE /v1/user/favorites/:favorite_id).
package delete_favorite

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

type FavoriteRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Favorite, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// =============================================================================
// UseCase
// =============================================================================

type UseCaseDeps struct {
	FavoriteRepo FavoriteRepo
}

type UseCase struct {
	repo FavoriteRepo
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{repo: deps.FavoriteRepo}
}

// Execute elimina un favorito previa verificación de ownership.
func (uc *UseCase) Execute(ctx context.Context, userIDString, favoriteIDString string) error {
	userID, err := uuid.Parse(userIDString)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	favoriteID, err := uuid.Parse(favoriteIDString)
	if err != nil {
		return fmt.Errorf("invalid favorite_id: %w", err)
	}

	// Verificar ownership
	existing, err := uc.repo.GetByID(ctx, favoriteID)
	if err != nil {
		if errors.Is(err, domain.ErrFavoriteNotFound) {
			return err
		}
		return fmt.Errorf("get favorite: %w", err)
	}
	if existing.UserID != userID {
		return domain.ErrFavoriteNotFound
	}

	if err := uc.repo.Delete(ctx, favoriteID); err != nil {
		return fmt.Errorf("delete favorite: %w", err)
	}

	return nil
}
