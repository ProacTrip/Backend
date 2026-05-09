// Caso de uso: Crear búsqueda guardada (POST /v1/user/saved-searches).
// Calcula el hash de parámetros (blake3) para deduplicación.
package create_saved_search

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

type SavedSearchRepo interface {
	Create(ctx context.Context, search *domain.SavedSearch) error
	GetByHash(ctx context.Context, userID uuid.UUID, searchHash string) (*domain.SavedSearch, error)
}

// =============================================================================
// Response
// =============================================================================

type Response struct {
	SearchID string `json:"search_id"`
	Message  string `json:"message"`
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

// Execute crea una búsqueda guardada con deduplicación por hash de parámetros.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// 1. Parsear parámetros a map para calcular el hash
	var paramsMap map[string]any
	if err := json.Unmarshal(cmd.Parameters, &paramsMap); err != nil {
		return nil, fmt.Errorf("unmarshal parameters: %w", err)
	}

	// 2. Calcular hash de dedup
	searchHash := domain.GenerateSearchHash(paramsMap)
	if searchHash == "" {
		return nil, fmt.Errorf("failed to generate search hash")
	}

	// 3. Verificar duplicado por (user_id, search_hash)
	existing, err := uc.repo.GetByHash(ctx, userID, searchHash)
	if err != nil {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}
	if existing != nil {
		return nil, domain.ErrDuplicateSavedSearch
	}

	// 4. Construir entidad
	now := time.Now()
	alertEnabled := false
	if cmd.AlertEnabled != nil {
		alertEnabled = *cmd.AlertEnabled
	}

	var filters json.RawMessage
	if len(cmd.Filters) > 0 {
		filters = cmd.Filters
	}

	search := &domain.SavedSearch{
		ID:                uuid.Must(uuid.NewV7()),
		UserID:            userID,
		Name:              cmd.Name,
		Parameters:        cmd.Parameters,
		Filters:           filters,
		SearchHash:        searchHash,
		SearchType:        cmd.SearchType,
		ParametersVersion: 1,
		AlertEnabled:      alertEnabled,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// 5. Persistir
	if err := uc.repo.Create(ctx, search); err != nil {
		return nil, fmt.Errorf("create saved search: %w", err)
	}

	return &Response{
		SearchID: search.ID.String(),
		Message:  "Búsqueda guardada creada correctamente.",
	}, nil
}
