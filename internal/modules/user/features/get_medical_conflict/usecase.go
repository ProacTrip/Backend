// Caso de uso: Obtener detalle de un conflicto médico.
package get_medical_conflict

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

// MedicalPendingRepo permite obtener conflictos médicos por ID.
type MedicalPendingRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	MedicalPendingRepo MedicalPendingRepo
}

// UseCase implementa la consulta de detalle de conflicto médico.
type UseCase struct {
	medicalPendingRepo MedicalPendingRepo
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		medicalPendingRepo: deps.MedicalPendingRepo,
	}
}

// Execute obtiene el detalle de un conflicto médico específico.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*MedicalConflictResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	conflictID, err := uuid.Parse(cmd.ConflictID)
	if err != nil {
		return nil, fmt.Errorf("invalid conflict_id: %w", err)
	}

	pu, err := uc.medicalPendingRepo.GetByID(ctx, conflictID)
	if err != nil {
		return nil, err
	}
	if pu == nil {
		return nil, domain.ErrPendingUpdateNotFound
	}

	// Verificar ownership
	if pu.UserID != userID {
		return nil, domain.ErrPendingUpdateNotFound
	}

	resp := &MedicalConflictResponse{
		ID:            pu.ID.String(),
		Field:         pu.FieldName,
		CurrentValue:  pu.CurrentValue,
		ProposedValue: pu.ProposedValue,
		Status:        string(pu.Status),
		SuggestedAt:   formatTime(pu.SuggestedAt),
		ExpiresAt:     formatTime(pu.ExpiresAt),
	}

	if pu.ResolvedAt != nil {
		s := formatTime(*pu.ResolvedAt)
		resp.ResolvedAt = &s
	}

	// Source
	if pu.SourceDocumentID != nil || pu.SourceFileName != nil {
		resp.Source = ConflictSource{
			Type: pu.SourceType,
		}
		if pu.SourceDocumentID != nil {
			sid := pu.SourceDocumentID.String()
			resp.Source.DocumentID = &sid
		}
		if pu.SourceFileName != nil {
			resp.Source.FileName = pu.SourceFileName
		}
	}

	return resp, nil
}

// formatTime retorna un time.Time como ISO 8601 string.
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
