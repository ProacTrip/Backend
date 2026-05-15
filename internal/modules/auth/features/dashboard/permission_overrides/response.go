// DTOs de respuesta para permission overrides del dashboard.
package permission_overrides

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// OverrideResponse — item individual
// =============================================================================

// OverrideResponse es el DTO de respuesta para un override de permiso.
type OverrideResponse struct {
	ID         uuid.UUID  `json:"id"`
	Permission string     `json:"permission"`
	Granted    bool       `json:"granted"`
	Reason     string     `json:"reason"`
	ExpiresAt  *time.Time `json:"expires_at,omitzero"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// =============================================================================
// OverrideListResponse — lista de overrides
// =============================================================================

// OverrideListResponse es la respuesta para listados de overrides de un usuario.
type OverrideListResponse struct {
	Overrides []OverrideResponse `json:"overrides"`
}

// =============================================================================
// OverrideRow — forma de scan de DB (privado al usecase)
// =============================================================================

// OverrideRow es el resultado de scan de DB para overrides de permiso.
// PO-SPEC-002: incluye actor metadata (created_by, updated_by).
type OverrideRow struct {
	ID         uuid.UUID
	Permission string
	Granted    bool
	Reason     string
	ExpiresAt  *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
