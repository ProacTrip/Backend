// RED — Test del usecase list_pending_medical.
package list_pending_medical

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockMedicalPendingRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]*domain.MedicalPendingUpdate, error)
}

func (m *mockMedicalPendingRepo) Create(ctx context.Context, u *domain.MedicalPendingUpdate) error {
	return nil
}
func (m *mockMedicalPendingRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.MedicalPendingUpdate, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockMedicalPendingRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
	return nil, nil
}
func (m *mockMedicalPendingRepo) Resolve(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error {
	return nil
}
func (m *mockMedicalPendingRepo) CountPending(ctx context.Context, userID uuid.UUID) (int, error) {
	return 0, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestListPendingMedical_HappyPath(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())
	now := time.Now()
	expires := now.Add(30 * 24 * time.Hour)

	updates := []*domain.MedicalPendingUpdate{
		{
			ID:               uuid.Must(uuid.NewV7()),
			UserID:           userID,
			SourceType:       "ocr",
			SourceDocumentID: &docID,
			FieldName:        "blood_type",
			CurrentValue:     new("A+"),
			ProposedValue:    "O+",
			SuggestedAt:      now,
			ExpiresAt:        expires,
			Status:           domain.PendingUpdatePending,
		},
		{
			ID:            uuid.Must(uuid.NewV7()),
			UserID:        userID,
			SourceType:    "ocr",
			FieldName:     "allergies",
			CurrentValue:  new("Penicilina"),
			ProposedValue: "Penicilina, Sulfa",
			SuggestedAt:   now,
			ExpiresAt:     expires,
			Status:        domain.PendingUpdatePending,
		},
	}

	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) ([]*domain.MedicalPendingUpdate, error) {
				return updates, nil
			},
		},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.Conflicts) != 2 {
		t.Errorf("conflicts = %d, se esperaban 2", len(resp.Conflicts))
	}

	// Verificar primer conflicto
	c1 := resp.Conflicts[0]
	if c1.Field != "blood_type" {
		t.Errorf("conflict[0].field = %s, se esperaba blood_type", c1.Field)
	}
	if c1.ProposedValue != "O+" {
		t.Errorf("conflict[0].proposed_value = %s, se esperaba O+", c1.ProposedValue)
	}
	if c1.CurrentValue == nil || *c1.CurrentValue != "A+" {
		t.Errorf("conflict[0].current_value = %v, se esperaba A+", c1.CurrentValue)
	}
	if c1.Source.Type != "ocr" {
		t.Errorf("conflict[0].source.type = %s, se esperaba ocr", c1.Source.Type)
	}

	// Verificar segundo conflicto
	c2 := resp.Conflicts[1]
	if c2.Field != "allergies" {
		t.Errorf("conflict[1].field = %s, se esperaba allergies", c2.Field)
	}
	if c2.ProposedValue != "Penicilina, Sulfa" {
		t.Errorf("conflict[1].proposed_value = %s, se esperaba Penicilina, Sulfa", c2.ProposedValue)
	}
}

func TestListPendingMedical_Empty(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) ([]*domain.MedicalPendingUpdate, error) {
				return []*domain.MedicalPendingUpdate{}, nil
			},
		},
	})

	cmd := Command{UserID: uuid.Must(uuid.NewV7()).String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.Conflicts) != 0 {
		t.Errorf("conflicts = %d, se esperaba 0", len(resp.Conflicts))
	}
}

func TestListPendingMedical_NilUpdates(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) ([]*domain.MedicalPendingUpdate, error) {
				return nil, nil
			},
		},
	})

	cmd := Command{UserID: uuid.Must(uuid.NewV7()).String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if resp.Conflicts == nil {
		t.Error("conflicts no debería ser nil, debería ser array vacío")
	}
	if len(resp.Conflicts) != 0 {
		t.Errorf("conflicts = %d, se esperaba 0", len(resp.Conflicts))
	}
}
