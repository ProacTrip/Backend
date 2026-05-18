// Lógica de negocio para habilitar/deshabilitar cuentas desde el dashboard.
// Orquesta validación, actualización DB, token_version++, e invalidación de sesiones.
package account_status

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/shared/session"
)

// =============================================================================
// Puerto de repositorio — interfaz local que el adapter PG implementa
// =============================================================================

// AccountStatusRepo es el puerto local para operaciones de estado de cuenta.
// Implementado por el adapter postgres.
type AccountStatusRepo interface {
	// GetByID obtiene un usuario por su ID.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)

	// UpdateStatus actualiza el estado del usuario y retorna el nuevo token_version.
	// La query: UPDATE users SET status = $1, token_version = token_version + 1,
	// updated_at = NOW() WHERE id = $2 RETURNING token_version.
	// El incremento de token_version es ATÓMICO (UPDATE ... RETURNING).
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) (int, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orquesta el cambio de estado de cuenta con invalidación de sesiones.
type UseCase struct {
	repo AccountStatusRepo
	rdb  *redis.Client
}

// NewUseCase crea un nuevo use case de account status.
func NewUseCase(repo AccountStatusRepo, rdb *redis.Client) *UseCase {
	return &UseCase{repo: repo, rdb: rdb}
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute ejecuta el cambio de estado.
// Flow: validate → get user → check self-disable → check no-op → DB update →
//
//	IF status=="disabled": invalidar sesiones (best-effort) → response.
//
// AS-SPEC-003: solo transiciones active↔disabled.
// AS-SPEC-005: token_version++ + sesiones invalidadas en disable.
func (uc *UseCase) Execute(ctx context.Context, cmd EnableDisableCommand) (*Response, error) {
	// 1. Validar
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Obtener usuario actual
	user, err := uc.repo.GetByID(ctx, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user for status change: %w", err)
	}

	previousStatus := string(user.Status)

	// 3. No permitir deshabilitarse a sí mismo
	if cmd.UserID == cmd.ActorID && cmd.Status == "disabled" {
		return nil, domain.ErrCannotDisableSelf
	}

	// 4. No-op: el status ya es el mismo
	if previousStatus == cmd.Status {
		return nil, fmt.Errorf("%w: user already has status %q", domain.ErrInvalidInput, cmd.Status)
	}

	// 5. Actualizar en DB (token_version++ atómico dentro del UPDATE)
	newTV, err := uc.repo.UpdateStatus(ctx, cmd.UserID, cmd.Status)
	if err != nil {
		return nil, fmt.Errorf("update user status: %w", err)
	}

	// 6. Invalidar sesiones cacheadas (disable Y enable)
	// AS-SPEC-005: token_version++ es la defensa primaria en disable.
	// En enable también invalidamos para que el cache no siga diciendo "disabled".
	// Best-effort: si falla, el TTL del cache (1 min) lo resuelve eventualmente.
	sessionsInvalidated := 0
	if cmd.Status == "disabled" || cmd.Status == "active" {
		sessionsInvalidated = uc.invalidateSessions(ctx, cmd.UserID)
	}

	return &Response{
		UserID:              cmd.UserID,
		PreviousStatus:      previousStatus,
		NewStatus:           cmd.Status,
		TokenVersion:        newTV,
		SessionsInvalidated: sessionsInvalidated,
	}, nil
}

// =============================================================================
// Invalidador de sesiones (best-effort)
// =============================================================================

// invalidateSessions invalida todas las sesiones cacheadas en Dragonfly.
// Retorna 0 si falla o no hay sesiones. Es best-effort: nunca revienta la operación.
func (uc *UseCase) invalidateSessions(ctx context.Context, userID uuid.UUID) int {
	// Sin cliente Redis → sin invalidación (no es error)
	if uc.rdb == nil {
		return 0
	}

	if err := session.InvalidateAllUserSessions(ctx, uc.rdb, userID.String()); err != nil {
		// Log pero no fallar — token_version mismatch es la defensa primaria
		_ = err
		return 0
	}

	// Éxito, pero no sabemos cuántas sesiones había exactamente.
	// Retornamos 1 como indicador de que se intentó la invalidación.
	return 1
}
