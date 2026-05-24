// Tests del usecase de verificación de documentos.
// DV-REQ-1: lectura con historial.
// DV-REQ-2: cambio de estado con audit trail.
package document_verification_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	docverification "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/document_verification"
)

// =============================================================================
// Stub / Mock
// =============================================================================

type stubDocVerificationRepo struct {
	getByID          func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
	getByIDForUpdate func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
	getHistory       func(ctx context.Context, docID uuid.UUID) ([]domain.HistoryEntry, error)
	insertHistory    func(ctx context.Context, entry domain.HistoryEntry) error
	updateStatus     func(ctx context.Context, id uuid.UUID, status string) error
}

func (s *stubDocVerificationRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
	return s.getByID(ctx, id)
}

func (s *stubDocVerificationRepo) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
	return s.getByIDForUpdate(ctx, id)
}

func (s *stubDocVerificationRepo) GetHistory(ctx context.Context, docID uuid.UUID) ([]domain.HistoryEntry, error) {
	return s.getHistory(ctx, docID)
}

func (s *stubDocVerificationRepo) InsertHistory(ctx context.Context, entry domain.HistoryEntry) error {
	return s.insertHistory(ctx, entry)
}

func (s *stubDocVerificationRepo) UpdateVerificationStatus(ctx context.Context, id uuid.UUID, status string) error {
	return s.updateStatus(ctx, id, status)
}

// =============================================================================
// Fixture
// =============================================================================

func testDocRow(id uuid.UUID, status string) *domain.DocumentRow {
	return &domain.DocumentRow{
		ID:                 id,
		UserID:             uuid.Must(uuid.NewV7()),
		VerificationStatus: status,
		OCRStatus:          "completed",
		DocumentTypeCode:   "passport",
		FileName:           "passport.pdf",
	}
}

// =============================================================================
// GET Tests
// =============================================================================

// TestExecute_WithHistory — DV-1.1: documento con historial previo.
func TestExecute_WithHistory(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "verified"), nil
		},
		getHistory: func(ctx context.Context, docID uuid.UUID) ([]domain.HistoryEntry, error) {
			return []domain.HistoryEntry{
				{NewStatus: "verified", VerifiedBy: uuid.Must(uuid.NewV7())},
			}, nil
		},
	}

	uc := docverification.NewUseCase(repo)
	cmd := docverification.VerifyCommand{DocumentID: docID}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "verified" {
		t.Errorf("status = %s, expected verified", resp.Status)
	}
	if len(resp.History) != 1 {
		t.Errorf("history len = %d, expected 1", len(resp.History))
	}
	if resp.VerifiedBy == nil {
		t.Error("verified_by should not be nil when history exists")
	}
}

// TestExecute_NeverVerified — DV-1.2: documento sin historial.
func TestExecute_NeverVerified(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "unverified"), nil
		},
		getHistory: func(ctx context.Context, docID uuid.UUID) ([]domain.HistoryEntry, error) {
			return []domain.HistoryEntry{}, nil
		},
	}

	uc := docverification.NewUseCase(repo)
	cmd := docverification.VerifyCommand{DocumentID: docID}
	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "unverified" {
		t.Errorf("status = %s, expected unverified", resp.Status)
	}
	if len(resp.History) != 0 {
		t.Errorf("history len = %d, expected 0", len(resp.History))
	}
	if resp.VerifiedBy != nil {
		t.Errorf("verified_by = %v, expected nil for never-verified doc", resp.VerifiedBy)
	}
}

// TestExecute_NotFound — DV-1.3: documento no encontrado.
func TestExecute_NotFound(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return nil, domain.ErrDocumentNotFound
		},
	}

	uc := docverification.NewUseCase(repo)
	cmd := docverification.VerifyCommand{DocumentID: docID}
	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for non-existent document, got nil")
	}
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// =============================================================================
// PATCH Tests
// =============================================================================

// TestExecuteUpdate_Approve — DV-2.1: aprobar documento con historial.
func TestExecuteUpdate_Approve(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	var insertedHistory domain.HistoryEntry

	repo := &stubDocVerificationRepo{
		getByIDForUpdate: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "unverified"), nil
		},
		updateStatus: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
		insertHistory: func(ctx context.Context, entry domain.HistoryEntry) error {
			insertedHistory = entry
			return nil
		},
	}

	uc := docverification.NewUseCase(repo)
	cmd := docverification.VerifyStatusCommand{
		DocumentID: docID,
		Status:     "verified",
		VerifiedBy: adminID,
	}

	resp, err := uc.ExecuteUpdate(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "verified" {
		t.Errorf("status = %s, expected verified", resp.Status)
	}
	if insertedHistory.PreviousStatus != "unverified" {
		t.Errorf("previous_status = %s, expected unverified", insertedHistory.PreviousStatus)
	}
	if insertedHistory.NewStatus != "verified" {
		t.Errorf("new_status = %s, expected verified", insertedHistory.NewStatus)
	}
	if insertedHistory.VerifiedBy != adminID {
		t.Errorf("verified_by = %s, expected %s", insertedHistory.VerifiedBy, adminID)
	}
}

// TestExecuteUpdate_Reject — DV-2.2: rechazar documento.
func TestExecuteUpdate_Reject(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByIDForUpdate: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "unverified"), nil
		},
		updateStatus: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
		insertHistory: func(ctx context.Context, entry domain.HistoryEntry) error {
			return nil
		},
	}

	uc := docverification.NewUseCase(repo)
	cmd := docverification.VerifyStatusCommand{
		DocumentID: docID,
		Status:     "rejected",
		Reason:     ptr("Documento borroso"),
		VerifiedBy: adminID,
	}

	resp, err := uc.ExecuteUpdate(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "rejected" {
		t.Errorf("status = %s, expected rejected", resp.Status)
	}
}

// TestExecuteUpdate_NoOp — mismo estado es idempotente.
func TestExecuteUpdate_NoOp(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByIDForUpdate: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDocRow(docID, "verified"), nil // ya está verified
		},
	}

	uc := docverification.NewUseCase(repo)
	cmd := docverification.VerifyStatusCommand{
		DocumentID: docID,
		Status:     "verified",
		VerifiedBy: adminID,
	}

	resp, err := uc.ExecuteUpdate(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "verified" {
		t.Errorf("status = %s, expected verified", resp.Status)
	}
}

// TestExecuteUpdate_NotFound — DV-2.8: documento no existe.
func TestExecuteUpdate_NotFound(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())
	adminID := uuid.Must(uuid.NewV7())

	repo := &stubDocVerificationRepo{
		getByIDForUpdate: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return nil, domain.ErrDocumentNotFound
		},
	}

	uc := docverification.NewUseCase(repo)
	cmd := docverification.VerifyStatusCommand{
		DocumentID: docID,
		Status:     "verified",
		VerifiedBy: adminID,
	}

	_, err := uc.ExecuteUpdate(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for non-existent document, got nil")
	}
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}
