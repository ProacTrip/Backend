// DTO de respuesta para listado de usuarios del dashboard.
// Incluye meta de paginación con cursores opacos.
package list_users

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Response — API response DTO
// =============================================================================

// Response is the list users API response with cursor pagination meta.
// Matches DU-SPEC-005: meta must contain next_cursor, prev_cursor, has_next, limit.
type Response struct {
	Users []UserResponse `json:"users"`
	Meta  Meta           `json:"meta"`
}

// =============================================================================
// UserResponse — item del listado
// =============================================================================

// UserResponse is a user item in the list endpoint response.
// DU-SPEC-004: NEVER includes password_hash, oauth secrets, locked_until, or failed_attempts.
type UserResponse struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	Status        string    `json:"status"`
	RoleID        uuid.UUID `json:"role_id"`
	RoleName      string    `json:"role_name"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// =============================================================================
// Meta — metadatos de paginación
// =============================================================================

// Meta contains pagination metadata following DU-SPEC-005 contract.
// prev_cursor is nil on first page. next_cursor is nil on last page.
// has_next indicates if more results exist. limit is the requested page size.
type Meta struct {
	NextCursor *string `json:"next_cursor,omitzero"`
	PrevCursor *string `json:"prev_cursor,omitzero"`
	HasNext    bool    `json:"has_next"`
	Limit      int     `json:"limit"`
}

// =============================================================================
// UserRow — forma de scan de DB (privado al usecase)
// =============================================================================

// UserRow is the DB scan shape for list queries.
// Only selects safe fields — NEVER password_hash or sensitive columns.
type UserRow struct {
	ID            uuid.UUID
	Email         string
	Status        string
	RoleID        uuid.UUID
	RoleName      string
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
