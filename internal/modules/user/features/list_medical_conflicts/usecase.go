// Caso de uso: Listar conflictos médicos del usuario con filtro de status.
package list_medical_conflicts

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

// MedicalPendingRepo permite listar conflictos médicos.
type MedicalPendingRepo interface {
	ListByUserID(ctx context.Context, userID uuid.UUID, status *domain.MedicalPendingUpdateStatus) ([]*domain.MedicalPendingUpdate, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	MedicalPendingRepo MedicalPendingRepo
}

// UseCase implementa la consulta de conflictos médicos.
type UseCase struct {
	medicalPendingRepo MedicalPendingRepo
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		medicalPendingRepo: deps.MedicalPendingRepo,
	}
}

// Execute lista los conflictos médicos para el usuario, con filtro de status opcional.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*ListMedicalConflictsResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// Parsear status filter si se proporcionó
	var status *domain.MedicalPendingUpdateStatus
	if cmd.Status != nil && *cmd.Status != "" {
		s := domain.MedicalPendingUpdateStatus(*cmd.Status)
		status = &s
	}

	updates, err := uc.medicalPendingRepo.ListByUserID(ctx, userID, status)
	if err != nil {
		return nil, fmt.Errorf("list medical conflicts: %w", err)
	}

	conflicts := make([]ConflictEntry, 0, len(updates))
	for _, pu := range updates {
		source := ConflictSource{
			Type: pu.SourceType,
		}

		if pu.SourceDocumentID != nil {
			sid := pu.SourceDocumentID.String()
			source.DocumentID = &sid
		}

		if pu.SourceFileName != nil {
			source.FileName = pu.SourceFileName
		}

		entry := ConflictEntry{
			ID:            pu.ID.String(),
			Field:         pu.FieldName,
			CurrentValue:  pu.CurrentValue,
			ProposedValue: pu.ProposedValue,
			Source:        source,
			Status:        string(pu.Status),
			SuggestedAt:   formatTime(pu.SuggestedAt),
			ExpiresAt:     formatTime(pu.ExpiresAt),
		}
		conflicts = append(conflicts, entry)
	}

	return &ListMedicalConflictsResponse{Conflicts: conflicts}, nil
}

// formatTime retorna un time.Time como ISO 8601 string.
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
