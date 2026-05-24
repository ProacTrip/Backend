// Lógica de negocio para verificación de documentos del dashboard.
// Orquesta GET (lectura de estado + historial) y PATCH (cambio de estado con audit trail).
package document_verification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Puerto de repositorio — interfaz local que el adapter PG implementa
// =============================================================================

// DocumentVerificationRepo es el puerto local para operaciones de verificación.
// Implementado por el adapter postgres.DocumentRepository.
type DocumentVerificationRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
	GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
	GetHistory(ctx context.Context, documentID uuid.UUID) ([]domain.HistoryEntry, error)
	InsertHistory(ctx context.Context, entry domain.HistoryEntry) error
	UpdateVerificationStatus(ctx context.Context, id uuid.UUID, status string) error
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orquesta la verificación de documentos con audit trail inmutable.
type UseCase struct {
	repo DocumentVerificationRepo
}

// NewUseCase crea un nuevo use case de verificación de documentos.
func NewUseCase(repo DocumentVerificationRepo) *UseCase {
	return &UseCase{repo: repo}
}

// =============================================================================
// Execute — GET: lectura de estado + historial
// =============================================================================

// Execute ejecuta la consulta de estado de verificación.
// Flow: validate → GetByID → GetHistory → VerificationResponse.
// DV-REQ-1: retorna estado actual + historial completo.
// DV-1.2: documento sin historial → status actual, history = [].
func (uc *UseCase) Execute(ctx context.Context, cmd VerifyCommand) (*VerificationResponse, error) {
	// 1. Validar
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Obtener documento
	doc, err := uc.repo.GetByID(ctx, cmd.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("get document for verification: %w", err)
	}

	// 3. Obtener historial
	historyEntries, err := uc.repo.GetHistory(ctx, cmd.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}

	// 4. Construir respuesta
	resp := &VerificationResponse{
		DocumentID: doc.ID,
		Status:     doc.VerificationStatus,
		History:    make([]HistoryEntryDTO, 0, len(historyEntries)),
	}

	// Si hay historial, el verified_by y verified_at vienen de la entrada más reciente
	if len(historyEntries) > 0 {
		latest := historyEntries[0] // ordenadas por changed_at DESC
		resp.VerifiedBy = &latest.VerifiedBy
		resp.VerifiedAt = &latest.ChangedAt
	}

	// Mapear domain.HistoryEntry → HistoryEntryDTO
	for _, entry := range historyEntries {
		resp.History = append(resp.History, HistoryEntryDTO{
			Status:     entry.NewStatus,
			VerifiedBy: entry.VerifiedBy,
			Reason:     entry.Reason,
			ChangedAt:  entry.ChangedAt,
		})
	}

	return resp, nil
}

// =============================================================================
// ExecuteUpdate — PATCH: cambio de estado con audit trail
// =============================================================================

// ExecuteUpdate ejecuta el cambio de estado de verificación.
// Flow: validate → GetByIDForUpdate (row lock) → validar estado no repetido →
//
//	UpdateVerificationStatus → InsertHistory → StatusResponse.
//
// DV-REQ-2: status solo puede ser verified|rejected|manual_review|suspicious.
// DV-REQ-3: cada cambio inserta una fila inmutable en document_verification_history.
// El row lock (SELECT ... FOR UPDATE) previene race conditions entre admins.
func (uc *UseCase) ExecuteUpdate(ctx context.Context, cmd VerifyStatusCommand) (*StatusResponse, error) {
	// 1. Validar
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Obtener documento con row lock (FOR UPDATE)
	doc, err := uc.repo.GetByIDForUpdate(ctx, cmd.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("get document for update: %w", err)
	}

	previousStatus := doc.VerificationStatus

	// 3. No-op: el status ya es el mismo
	if previousStatus == cmd.Status {
		return &StatusResponse{
			DocumentID: cmd.DocumentID,
			Status:     cmd.Status,
			Message:    "El documento ya tiene este estado de verificación",
		}, nil
	}

	// 4. Actualizar verification_status en user_documents
	if err := uc.repo.UpdateVerificationStatus(ctx, cmd.DocumentID, cmd.Status); err != nil {
		return nil, fmt.Errorf("update verification status: %w", err)
	}

	// 5. Insertar entrada inmutable en el historial (DV-REQ-3)
	historyEntry := domain.HistoryEntry{
		DocumentID:     cmd.DocumentID,
		PreviousStatus: previousStatus,
		NewStatus:      cmd.Status,
		VerifiedBy:     cmd.VerifiedBy,
		Reason:         cmd.Reason,
		ChangedAt:      time.Now(),
	}

	if err := uc.repo.InsertHistory(ctx, historyEntry); err != nil {
		return nil, fmt.Errorf("insert history entry: %w", err)
	}

	return &StatusResponse{
		DocumentID: cmd.DocumentID,
		Status:     cmd.Status,
		Message:    "Estado de verificación actualizado correctamente",
	}, nil
}
