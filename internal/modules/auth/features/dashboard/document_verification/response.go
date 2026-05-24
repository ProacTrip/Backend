// DTO de respuesta para verificación de documentos.
// GET → VerificationResponse (con historial completo).
// PATCH → StatusResponse (confirmación).
package document_verification

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// VerificationResponse — respuesta del GET /documents/:id/verification
// =============================================================================

// VerificationResponse contiene el estado actual y el historial completo.
// DV-REQ-1: document_id, status, verified_by, verified_at, history[].
type VerificationResponse struct {
	DocumentID uuid.UUID         `json:"document_id"`
	Status     string            `json:"status"`
	VerifiedBy *uuid.UUID        `json:"verified_by,omitzero"`
	VerifiedAt *time.Time        `json:"verified_at,omitzero"`
	History    []HistoryEntryDTO `json:"history"`
}

// =============================================================================
// StatusResponse — respuesta del PATCH /documents/:id/verification
// =============================================================================

// StatusResponse confirma el cambio de estado de verificación.
type StatusResponse struct {
	DocumentID uuid.UUID `json:"document_id"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
}

// =============================================================================
// HistoryEntryDTO — entrada del historial de verificación
// =============================================================================

// HistoryEntryDTO representa una entrada del historial de verificación en la API.
// Incluye Status (equivalente a NewStatus del domain), VerifiedBy, Reason y ChangedAt.
type HistoryEntryDTO struct {
	Status     string    `json:"status"`
	VerifiedBy uuid.UUID `json:"verified_by"`
	Reason     *string   `json:"reason,omitzero"`
	ChangedAt  time.Time `json:"changed_at"`
}
