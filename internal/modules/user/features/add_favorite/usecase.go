// Caso de uso: Agregar favorito (POST /v1/user/favorites).
// Verifica constraint (user_id, entity_id, entity_type) único.
package add_favorite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

type FavoriteRepo interface {
	Create(ctx context.Context, fav *domain.Favorite) error
}

// =============================================================================
// Response
// =============================================================================

type Response struct {
	FavoriteID string `json:"favorite_id"`
	Message    string `json:"message"`
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

// Execute crea un favorito. La constraint unique (user_id, entity_id, entity_type)
// se valida a nivel de DB, devolviendo ErrDuplicateFavorite si ya existe.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	entityID, err := uuid.Parse(cmd.EntityID)
	if err != nil {
		return nil, fmt.Errorf("invalid entity_id: %w", err)
	}

	entityType := domain.FavoriteEntityType(cmd.EntityType)
	if !domain.IsValidFavoriteEntityType(cmd.EntityType) {
		return nil, domain.ErrInvalidFavoriteEntityType
	}
	now := time.Now()

	fav := &domain.Favorite{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		EntityID:   entityID,
		EntityType: entityType,
		Title:      cmd.Title,
		Notes:      cmd.Notes,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := uc.repo.Create(ctx, fav); err != nil {
		return nil, fmt.Errorf("create favorite: %w", err)
	}

	return &Response{
		FavoriteID: fav.ID.String(),
		Message:    "Agregado a favoritos.",
	}, nil
}
