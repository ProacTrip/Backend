// DTO de respuesta para reprocesar documento OCR.
// POST /v1/dashboard/documents/:id/reprocess → 202 Accepted
package document_reprocess

import "github.com/google/uuid"

// =============================================================================
// ReprocessResponse — respuesta 202 Accepted
// =============================================================================

// ReprocessResponse confirma que el documento fue encolado para reprocesamiento.
// DR-REQ-1: document_id, status="queued", message descriptivo.
type ReprocessResponse struct {
	DocumentID uuid.UUID `json:"document_id"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
}
