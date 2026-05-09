// Caso de uso: Alternar alerta de precio (PUT /v1/user/saved-searches/:search_id/alert).
package toggle_alert

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
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error)
	SetAlertEnabled(ctx context.Context, id uuid.UUID, enabled bool) error
}

// =============================================================================
// Response
// =============================================================================

type Response struct {
	SearchID     string `json:"search_id"`
	AlertEnabled bool   `json:"alert_enabled"`
	Message      string `json:"message"`
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

// Execute activa o desactiva la alerta de precio para una búsqueda guardada.
// El worker de alerta de precio está DEFERRED — este endpoint solo cambia el flag.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	searchID, err := uuid.Parse(cmd.SearchID)
	if err != nil {
		return nil, fmt.Errorf("invalid search_id: %w", err)
	}

	// Verificar ownership
	existing, err := uc.repo.GetByID(ctx, searchID)
	if err != nil {
		return nil, err
	}
	if existing.UserID != userID {
		return nil, domain.ErrSearchNotFound
	}

	if existing.AlertEnabled != cmd.Enabled {
		if err := uc.repo.SetAlertEnabled(ctx, searchID, cmd.Enabled); err != nil {
			return nil, fmt.Errorf("set alert enabled: %w", err)
		}
		existing.AlertEnabled = cmd.Enabled
	}

	msg := "Alerta de precio desactivada."
	if existing.AlertEnabled {
		msg = "Alerta de precio activada correctamente."
	}

	return &Response{
		SearchID:     searchID.String(),
		AlertEnabled: existing.AlertEnabled,
		Message:      msg,
	}, nil
}
