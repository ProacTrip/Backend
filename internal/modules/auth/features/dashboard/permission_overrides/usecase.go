// Lógica de negocio para CRUD de permission overrides desde el dashboard.
// Orquesta validación, operaciones DB e invalidación de sesiones.
package permission_overrides

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/shared/session"
)

// =============================================================================
// Puerto de repositorio — interfaz local que el adapter PG implementa
// =============================================================================

// OverrideRepo es el puerto local para operaciones CRUD de permission overrides.
// Implementado por el adapter postgres.
type OverrideRepo interface {
	// GetOverridesByUserID retorna todos los overrides de un usuario.
	// PO-SPEC-002: incluye overrides expirados (el cliente o resolver los filtra).
	GetOverridesByUserID(ctx context.Context, userID uuid.UUID) ([]OverrideRow, error)

	// CreateOverride crea un nuevo override y retorna su ID.
	// PO-SPEC-001: actor tracking via createdBy.
	CreateOverride(ctx context.Context, userID, permissionID uuid.UUID, granted bool, expiresAt *time.Time, reason string, createdBy uuid.UUID) (uuid.UUID, error)

	// UpdateOverride actualiza un override existente.
	// PO-SPEC-008: updatedBy set al actor actual.
	UpdateOverride(ctx context.Context, overrideID uuid.UUID, granted bool, expiresAt *time.Time, reason string, updatedBy uuid.UUID) error

	// DeleteOverride elimina un override.
	DeleteOverride(ctx context.Context, overrideID uuid.UUID) error
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orquesta el CRUD de permission overrides con invalidación de sesiones.
type UseCase struct {
	repo OverrideRepo
	rdb  *redis.Client
}

// NewUseCase crea un nuevo use case de permission overrides.
func NewUseCase(repo OverrideRepo, rdb *redis.Client) *UseCase {
	return &UseCase{repo: repo, rdb: rdb}
}

// =============================================================================
// List Overrides
// =============================================================================

// ListOverrides lista todos los overrides de un usuario.
// PO-SPEC-002: incluye expirados (el cliente los filtra).
func (uc *UseCase) ListOverrides(ctx context.Context, cmd ListOverridesCommand) (*OverrideListResponse, error) {
	rows, err := uc.repo.GetOverridesByUserID(ctx, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("list permission overrides: %w", err)
	}

	overrides := make([]OverrideResponse, len(rows))
	for i, row := range rows {
		overrides[i] = OverrideResponse{
			ID:         row.ID,
			Permission: row.Permission,
			Granted:    row.Granted,
			Reason:     row.Reason,
			ExpiresAt:  row.ExpiresAt,
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		}
	}

	return &OverrideListResponse{Overrides: overrides}, nil
}

// =============================================================================
// Create Override
// =============================================================================

// CreateOverride crea un override de permiso (grant o deny).
// PO-SPEC-001: actor tracking via createdBy, razón requerida, expiración opcional.
// PO-SPEC-006: razón validada (no vacía, no solo whitespace, 1–500 chars).
// PO-SPEC-007: deny con expiración > 365 días → ErrInvalidBlockDuration.
// PO-SPEC-004: invalida sesiones cacheadas del usuario (best-effort).
func (uc *UseCase) CreateOverride(ctx context.Context, cmd CreateOverrideCommand) (*OverrideResponse, error) {
	// 1. Validar
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Crear override en DB (con actor tracking)
	overrideID, err := uc.repo.CreateOverride(ctx, cmd.UserID, cmd.PermissionID, cmd.Granted, cmd.ExpiresAt, cmd.Reason, cmd.ActorID)
	if err != nil {
		return nil, fmt.Errorf("create permission override: %w", err)
	}

	// 3. Invalidar sesiones cacheadas del usuario afectado (best-effort)
	// PO-SPEC-004: cache miss en próximo request → DB fallback con overrides actualizados.
	uc.invalidateSessions(ctx, cmd.UserID)
	now := time.Now()
	return &OverrideResponse{
		ID:         overrideID,
		Permission: "", // se completa en el adapter con JOIN a permissions table
		Granted:    cmd.Granted,
		Reason:     cmd.Reason,
		ExpiresAt:  cmd.ExpiresAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// =============================================================================
// Delete Override
// =============================================================================

// DeleteOverride elimina un override y limpia la cache de sesiones del usuario.
// PO-SPEC-004: DELETE invalida sesiones cacheadas.
func (uc *UseCase) DeleteOverride(ctx context.Context, cmd DeleteOverrideCommand) error {
	if err := uc.repo.DeleteOverride(ctx, cmd.OverrideID); err != nil {
		return fmt.Errorf("delete permission override: %w", err)
	}

	// Invalidar sesiones cacheadas (best-effort)
	uc.invalidateSessions(ctx, cmd.UserID)
	return nil
}

// =============================================================================
// Invalidación de sesiones (best-effort)
// =============================================================================

// invalidateSessions invalida todas las sesiones cacheadas en Dragonfly.
// Best-effort: si falla, el token_version mismatch o lazy refresh lo resuelven.
func (uc *UseCase) invalidateSessions(ctx context.Context, userID uuid.UUID) {
	if uc.rdb == nil {
		return
	}

	if err := session.InvalidateAllUserSessions(ctx, uc.rdb, userID.String()); err != nil {
		// Log pero no fallar
		_ = err
	}
}
