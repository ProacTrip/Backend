// DTO de respuesta para detalle de usuario del dashboard.
// Incluye permisos efectivos computados por PermissionResolver.
package user_detail

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Response — API response DTO
// =============================================================================

// Response is the user detail API response.
// DU-SPEC-003: MUST include effective_permissions array.
// DU-SPEC-004: MUST NEVER include password_hash, oauth internals, locked_until, failed_attempts.
type Response struct {
	User                 UserDetailResponse `json:"user"`
	EffectivePermissions []string           `json:"effective_permissions"`
}

// =============================================================================
// UserDetailResponse — detalle del usuario sin campos sensibles
// =============================================================================

// UserDetailResponse contains safe user fields for the dashboard detail endpoint.
// Fields EXCLUDED (by design): password_hash, oauth_provider_id, locked_until, failed_attempts.
type UserDetailResponse struct {
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	Status        string     `json:"status"`
	RoleID        uuid.UUID  `json:"role_id"`
	RoleName      string     `json:"role_name"`
	EmailVerified bool       `json:"email_verified"`
	LoginCount    int        `json:"login_count"`
	LastLoginAt   *time.Time `json:"last_login_at,omitzero"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
