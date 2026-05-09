// Caso de uso: Listar búsquedas guardadas (GET /v1/user/saved-searches).
package list_saved_searches

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

type SavedSearchRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error)
}

// =============================================================================
// Response
// =============================================================================

type SearchItem struct {
	ID                string           `json:"id"`
	Name              *string          `json:"name,omitzero"`
	Parameters        any              `json:"parameters"`
	Filters           any              `json:"filters,omitzero"`
	SearchType        *string          `json:"search_type,omitzero"`
	ParametersVersion int              `json:"parameters_version"`
	AlertEnabled      bool             `json:"alert_enabled"`
	LastExecutedAt    *string          `json:"last_executed_at,omitzero"`
	ResultCount       *int             `json:"result_count,omitzero"`
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
}

type Response struct {
	Searches []SearchItem `json:"searches"`
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

// Execute devuelve todas las búsquedas guardadas del usuario.
func (uc *UseCase) Execute(ctx context.Context, userIDString string) (*Response, error) {
	userID, err := uuid.Parse(userIDString)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	searches, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list saved searches: %w", err)
	}

	items := make([]SearchItem, 0, len(searches))
	for _, s := range searches {
		item := SearchItem{
			ID:                s.ID.String(),
			Name:              s.Name,
			SearchType:        s.SearchType,
			ParametersVersion: s.ParametersVersion,
			AlertEnabled:      s.AlertEnabled,
			ResultCount:       s.ResultCount,
			CreatedAt:         s.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:         s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}

		// Deserializar parámetros y filtros para mostrarlos como objetos JSON
		if len(s.Parameters) > 0 {
			item.Parameters = s.Parameters
		}
		if len(s.Filters) > 0 {
			item.Filters = s.Filters
		}

		if s.LastExecutedAt != nil {
			formatted := s.LastExecutedAt.Format("2006-01-02T15:04:05Z")
			item.LastExecutedAt = &formatted
		}

		items = append(items, item)
	}

	return &Response{Searches: items}, nil
}
