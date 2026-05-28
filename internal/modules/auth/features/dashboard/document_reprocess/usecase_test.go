// Tests del usecase de reprocesamiento OCR.
// DR-REQ-1: reprocesar → 202, not found → 404, idempotente.
package document_reprocess_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	docreprocess "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/document_reprocess"
)

func strPtr(s string) *string { return &s }

// =============================================================================
// Stub
// =============================================================================

type stubDocReprocessRepo struct {
	getByID       func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
	updateOCR     func(ctx context.Context, id uuid.UUID, status string) error
}

func (s *stubDocReprocessRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
	return s.getByID(ctx, id)
}

func (s *stubDocReprocessRepo) UpdateOCRStatus(ctx context.Context, id uuid.UUID, status string) error {
	return s.updateOCR(ctx, id, status)
}

// =============================================================================
// Fixture
// =============================================================================

func testDoc(id uuid.UUID, ocrStatus string) *domain.DocumentRow {
	return &domain.DocumentRow{
		ID:                 id,
		UserID:             uuid.Must(uuid.NewV7()),
		VerificationStatus: "unverified",
		OCRStatus:          ocrStatus,
		DocumentTypeCode:   strPtr("passport"),
		FileName:           "doc.pdf",
		StorageKey:         "raw/" + id.String(),
	}
}

// =============================================================================
// Tests
// =============================================================================

// TestExecute_Reprocess — DR-1.1: reprocesar documento, retorna 202.
func TestExecute_Reprocess(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	var updatedStatus string

	repo := &stubDocReprocessRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDoc(docID, "failed"), nil
		},
		updateOCR: func(ctx context.Context, id uuid.UUID, status string) error {
			updatedStatus = status
			return nil
		},
	}

	uc := docreprocess.NewUseCase(repo, nil, nil)
	cmd := docreprocess.ReprocessCommand{
		DocumentID: docID,
		ActorID:    actorID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "queued" {
		t.Errorf("status = %s, expected queued", resp.Status)
	}
	if updatedStatus != "queued" {
		t.Errorf("updated ocr_status = %s, expected queued", updatedStatus)
	}
}

// TestExecute_NotFound — DR-1.2: documento no encontrado.
func TestExecute_NotFound(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubDocReprocessRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return nil, domain.ErrDocumentNotFound
		},
	}

	uc := docreprocess.NewUseCase(repo, nil, nil)
	cmd := docreprocess.ReprocessCommand{
		DocumentID: docID,
		ActorID:    actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for non-existent document, got nil")
	}
	if !errors.Is(err, domain.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// TestExecute_Idempotent — DR-1.3: documento ya en "queued" → mismo 202.
func TestExecute_Idempotent(t *testing.T) {
	ctx := t.Context()
	docID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	updateCalled := false

	repo := &stubDocReprocessRepo{
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
			return testDoc(docID, "queued"), nil
		},
		updateOCR: func(ctx context.Context, id uuid.UUID, status string) error {
			updateCalled = true
			return nil
		},
	}

	uc := docreprocess.NewUseCase(repo, nil, nil)
	cmd := docreprocess.ReprocessCommand{
		DocumentID: docID,
		ActorID:    actorID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "queued" {
		t.Errorf("status = %s, expected queued", resp.Status)
	}
	if updateCalled {
		t.Error("UpdateOCRStatus should NOT be called when already queued")
	}
}
