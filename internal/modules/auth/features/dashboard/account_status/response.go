// DTO de respuesta para enable/disable de cuenta.
// Incluye estados anterior y nuevo, token_version y conteo de sesiones invalidadas.
package account_status

import "github.com/google/uuid"

// =============================================================================
// Response — API response DTO
// =============================================================================

// Response es la respuesta del endpoint de cambio de estado.
// AS-SPEC-003: 200 con datos del usuario actualizado.
// Single-session: ya no incluye conteo de sesiones invalidadas.
type Response struct {
	UserID         uuid.UUID `json:"user_id"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	TokenVersion   int       `json:"token_version"`
}
