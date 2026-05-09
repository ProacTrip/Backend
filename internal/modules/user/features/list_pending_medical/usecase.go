// Caso de uso: Listar conflictos médicos pendientes del usuario.
package list_pending_medical

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// MedicalPendingRepo permite leer actualizaciones pendientes.
type MedicalPendingRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.MedicalPendingUpdate, error)
}

// =============================================================================
// Response types
// =============================================================================

// ConflictSource representa el origen de un conflicto.
type ConflictSource struct {
	Type       string  `json:"type"`
	DocumentID *string `json:"document_id,omitzero"`
	FileName   *string `json:"file_name,omitzero"`
}

// ConflictEntry representa un conflicto pendiente individual.
type ConflictEntry struct {
	ID            string         `json:"id"`
	Field         string         `json:"field"`
	CurrentValue  *string        `json:"current_value,omitzero"`
	ProposedValue string         `json:"proposed_value"`
	Source        ConflictSource `json:"source"`
	SuggestedAt   string         `json:"suggested_at"`
	ExpiresAt     string         `json:"expires_at"`
}

// ListPendingResponse es la respuesta del endpoint de conflictos pendientes.
type ListPendingResponse struct {
	Conflicts []ConflictEntry `json:"conflicts"`
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	MedicalPendingRepo MedicalPendingRepo
}

// UseCase implementa la consulta de conflictos médicos pendientes.
type UseCase struct {
	medicalPendingRepo MedicalPendingRepo
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		medicalPendingRepo: deps.MedicalPendingRepo,
	}
}

// Execute lista los conflictos médicos pendientes para el usuario.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*ListPendingResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	updates, err := uc.medicalPendingRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get pending updates: %w", err)
	}

	conflicts := make([]ConflictEntry, 0, len(updates))
	for _, pu := range updates {
		source := ConflictSource{
			Type: pu.SourceType,
		}

		// Document ID si existe
		if pu.SourceDocumentID != nil {
			sid := pu.SourceDocumentID.String()
			source.DocumentID = &sid
		}

		entry := ConflictEntry{
			ID:            pu.ID.String(),
			Field:         pu.FieldName,
			CurrentValue:  pu.CurrentValue,
			ProposedValue: pu.ProposedValue,
			Source:        source,
			SuggestedAt:   formatTime(pu.SuggestedAt),
			ExpiresAt:     formatTime(pu.ExpiresAt),
		}
		conflicts = append(conflicts, entry)
	}

	return &ListPendingResponse{Conflicts: conflicts}, nil
}

// formatTime retorna un time.Time como ISO 8601 string.
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
