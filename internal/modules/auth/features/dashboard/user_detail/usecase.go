// Lógica de negocio para detalle de usuario del dashboard.
// Orquesta búsqueda en DB y resolución de permisos efectivos.
package user_detail

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Puertos — interfaces locales que los adapters implementan
// =============================================================================

// UserDetailRepo is the local port for fetching a single user by ID.
// Implemented by the postgres adapter.
type UserDetailRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

// PermissionResolver is the local port for resolving effective permissions.
// Implemented by domain/services/permission_resolver.go.
type PermissionResolver interface {
	ResolveEffectivePermissions(ctx context.Context, userID, roleID uuid.UUID) ([]string, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orchestrates user detail retrieval with permission resolution.
type UseCase struct {
	repo     UserDetailRepo
	resolver PermissionResolver
}

// NewUseCase creates a new user detail use case.
func NewUseCase(repo UserDetailRepo, resolver PermissionResolver) *UseCase {
	return &UseCase{repo: repo, resolver: resolver}
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute performs user detail retrieval.
// Flow: validate → DB lookup → resolve effective permissions → respond.
// DU-SPEC-003: effective_permissions computed via PermissionResolver.
// DU-SPEC-004: NEVER returns password_hash, locked_until, failed_attempts.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Validate
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Fetch user from DB
	user, err := uc.repo.GetByID(ctx, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user detail: %w", err)
	}

	// 3. Resolve effective permissions via PermissionResolver
	// DU-SPEC-003: computed from (role_permissions ∪ active_grants) − active_denies
	permissions, err := uc.resolver.ResolveEffectivePermissions(ctx, user.ID, user.RoleID)
	if err != nil {
		return nil, fmt.Errorf("resolve permissions: %w", err)
	}

	// 4. Build safe response — DU-SPEC-004: exclude password_hash, locked_until, failed_attempts
	return &Response{
		User: UserDetailResponse{
			ID:            user.ID,
			Email:         user.Email,
			Status:        string(user.Status),
			RoleID:        user.RoleID,
			RoleName:      user.RoleName,
			EmailVerified: user.EmailVerified,
			MFAEnabled:    user.MFAEnabled,
			LoginCount:    user.LoginCount,
			LastLoginAt:   user.LastLoginAt,
			CreatedAt:     user.CreatedAt,
			UpdatedAt:     user.UpdatedAt,
		},
		EffectivePermissions: permissions,
	}, nil
}
