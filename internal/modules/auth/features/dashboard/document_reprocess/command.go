// DTO de entrada para reprocesar documento OCR desde el dashboard.
// POST /v1/dashboard/documents/:id/reprocess
package document_reprocess

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// ReprocessCommand
// =============================================================================

// ReprocessCommand es el DTO de entrada para re-ejecutar el pipeline OCR.
// DocumentID es extraído del path param por el handler.
// ActorID viene del PASETO claims (no del body).
type ReprocessCommand struct {
	DocumentID uuid.UUID
	ActorID    uuid.UUID
}

// Validate rechaza UUID nulo y actor no autenticado.
func (cmd *ReprocessCommand) Validate() error {
	if cmd.DocumentID == uuid.Nil {
		return fmt.Errorf("%w: document ID is required", domain.ErrInvalidInput)
	}
	if cmd.ActorID == uuid.Nil {
		return fmt.Errorf("%w: actor ID is required", domain.ErrInvalidInput)
	}
	return nil
}
