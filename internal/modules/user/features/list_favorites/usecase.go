// Caso de uso: Listar favoritos (GET /v1/user/favorites).
// Filtro opcional por entity_type.
package list_favorites

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

type FavoriteRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Favorite, error)
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

// Execute devuelve los favoritos del usuario, opcionalmente filtrados por entity_type.
func (uc *UseCase) Execute(ctx context.Context, userIDString string, entityTypeFilter *string) (*Response, error) {
	userID, err := uuid.Parse(userIDString)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	favs, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}

	items := make([]FavoriteItem, 0, len(favs))
	for _, f := range favs {
		// Filtrar por entity_type si se especificó
		if entityTypeFilter != nil && *entityTypeFilter != "" {
			if string(f.EntityType) != *entityTypeFilter {
				continue
			}
		}

		items = append(items, FavoriteItem{
			ID:         f.ID.String(),
			EntityID:   f.EntityID.String(),
			EntityType: string(f.EntityType),
			Title:      f.Title,
			Notes:      f.Notes,
			CreatedAt:  f.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return &Response{Favorites: items}, nil
}
