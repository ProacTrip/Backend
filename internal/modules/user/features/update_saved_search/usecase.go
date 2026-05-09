// Caso de uso: Actualizar búsqueda guardada (PUT /v1/user/saved-searches/:search_id).
// Recalcula el hash si los parámetros cambian, verifica dedup.
package update_saved_search

import (
	"context"
	"encoding/json"
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
	GetByHash(ctx context.Context, userID uuid.UUID, searchHash string) (*domain.SavedSearch, error)
	Update(ctx context.Context, search *domain.SavedSearch) error
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

// Execute actualiza una búsqueda guardada con merge parcial.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) error {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	searchID, err := uuid.Parse(cmd.SearchID)
	if err != nil {
		return fmt.Errorf("invalid search_id: %w", err)
	}

	// 1. Verificar que la búsqueda existe y pertenece al usuario
	existing, err := uc.repo.GetByID(ctx, searchID)
	if err != nil {
		return err
	}
	if existing.UserID != userID {
		return domain.ErrSearchNotFound
	}

	// 2. Construir la entidad para actualización parcial
	update := &domain.SavedSearch{
		ID:         searchID,
		Name:       cmd.Name,
		Filters:    existing.Filters,
		SearchHash: existing.SearchHash,
		SearchType: cmd.SearchType,
	}

	var newHash string
	if cmd.Parameters != nil && len(*cmd.Parameters) > 0 {
		// Recalcular hash si los parámetros cambiaron
		var paramsMap map[string]any
		if err := json.Unmarshal(*cmd.Parameters, &paramsMap); err != nil {
			return fmt.Errorf("unmarshal parameters: %w", err)
		}

		newHash = domain.GenerateSearchHash(paramsMap)
		if newHash == "" {
			return fmt.Errorf("failed to generate search hash")
		}

		// Verificar que el nuevo hash no colisione con otra búsqueda del mismo usuario
		if newHash != existing.SearchHash {
			collision, err := uc.repo.GetByHash(ctx, userID, newHash)
			if err != nil {
				return fmt.Errorf("check hash collision: %w", err)
			}
			if collision != nil && collision.ID != searchID {
				return domain.ErrDuplicateSavedSearch
			}
			update.SearchHash = newHash
		}
	}

	if cmd.Filters != nil {
		update.Filters = *cmd.Filters
	}
	if cmd.Parameters != nil {
		update.Parameters = *cmd.Parameters
	} else {
		update.Parameters = existing.Parameters
	}

	// 3. Persistir
	if err := uc.repo.Update(ctx, update); err != nil {
		if errors.Is(err, domain.ErrSearchNotFound) {
			return err
		}
		return fmt.Errorf("update saved search: %w", err)
	}

	return nil
}
