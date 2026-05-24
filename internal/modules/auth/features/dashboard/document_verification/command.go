// DTO de entrada para GET y PATCH de verificación de documentos.
// GET /v1/dashboard/documents/:id/verification
// PATCH /v1/dashboard/documents/:id/verification
package document_verification

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// VerifyCommand — entrada para GET (solo DocumentID)
// =============================================================================

// VerifyCommand es el DTO de entrada para consultar el estado de verificación.
// DocumentID es extraído del path param por el handler.
type VerifyCommand struct {
	DocumentID uuid.UUID
}

// Validate rechaza UUID nulo.
func (cmd *VerifyCommand) Validate() error {
	if cmd.DocumentID == uuid.Nil {
		return fmt.Errorf("%w: document ID is required", domain.ErrInvalidInput)
	}
	return nil
}

// =============================================================================
// VerifyStatusCommand — entrada para PATCH (DocumentID + Status + Reason + VerifiedBy)
// =============================================================================

// VerifyStatusCommand es el DTO de entrada para actualizar el estado de verificación.
// Status debe ser uno de: verified, rejected, manual_review, suspicious.
// Reason es opcional (máx 500 caracteres).
// VerifiedBy viene del PASETO claims, NUNCA del body.
type VerifyStatusCommand struct {
	DocumentID uuid.UUID
	Status     string
	Reason     *string // Máximo 500 caracteres
	VerifiedBy uuid.UUID
}

// Validate rechaza UUID nulo, status vacío, status no permitido, reason > 500 chars.
// DV-2.5: "pending" es un estado inicial de solo lectura, no permitido como destino.
// DV-2.6: reason no puede exceder 500 caracteres.
// DV-2.7: status es requerido.
func (cmd *VerifyStatusCommand) Validate() error {
	if cmd.DocumentID == uuid.Nil {
		return fmt.Errorf("%w: document ID is required", domain.ErrInvalidInput)
	}

	if cmd.Status == "" {
		return fmt.Errorf("%w: status is required", domain.ErrValidationError)
	}

	if cmd.VerifiedBy == uuid.Nil {
		return fmt.Errorf("%w: verified_by is required", domain.ErrInvalidInput)
	}

	if !isValidVerificationStatus(cmd.Status) {
		return fmt.Errorf("%w: invalid status %q — debe ser verified, rejected, manual_review o suspicious", domain.ErrValidationError, cmd.Status)
	}

	if cmd.Reason != nil && len(*cmd.Reason) > 500 {
		return fmt.Errorf("%w: reason no puede exceder 500 caracteres", domain.ErrInvalidInput)
	}

	return nil
}

// =============================================================================
// Validación de estados
// =============================================================================

// validVerificationStatuses contiene los estados permitidos para PATCH de verificación.
// "pending" NO está incluido — es estado inicial readonly (DV-2.5).
var validVerificationStatuses = map[string]bool{
	"verified":      true,
	"rejected":      true,
	"manual_review": true,
	"suspicious":    true,
}

// isValidVerificationStatus verifica si un status es válido para transición via dashboard.
func isValidVerificationStatus(s string) bool {
	return validVerificationStatuses[s]
}
