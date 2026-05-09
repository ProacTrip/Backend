// RED — Test del usecase resolve_medical_pending.
package resolve_medical_pending

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockMedicalPendingRepo struct {
	getByIDFn      func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error)
	resolveFn      func(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error
}

func (m *mockMedicalPendingRepo) Create(ctx context.Context, u *domain.MedicalPendingUpdate) error {
	return nil
}
func (m *mockMedicalPendingRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.MedicalPendingUpdate, error) {
	return nil, nil
}
func (m *mockMedicalPendingRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockMedicalPendingRepo) Resolve(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error {
	if m.resolveFn != nil {
		return m.resolveFn(ctx, id, status)
	}
	return nil
}
func (m *mockMedicalPendingRepo) CountPending(ctx context.Context, userID uuid.UUID) (int, error) {
	return 0, nil
}

type mockMedicalProfileRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfileV2, error)
	updateFn      func(ctx context.Context, profile *domain.MedicalProfileV2) error
}

func (m *mockMedicalProfileRepo) Create(ctx context.Context, p *domain.MedicalProfileV2) error { return nil }
func (m *mockMedicalProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfileV2, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockMedicalProfileRepo) Update(ctx context.Context, p *domain.MedicalProfileV2) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, p)
	}
	return nil
}

type mockEncryptionService struct{}

func (m *mockEncryptionService) Encrypt(plaintext string) ([]byte, error) {
	return []byte("enc:" + plaintext), nil
}
func (m *mockEncryptionService) Decrypt(ciphertext []byte) (string, error) {
	s := string(ciphertext)
	if strings.HasPrefix(s, "enc:") {
		return s[4:], nil
	}
	return s, nil
}

// =============================================================================
// Helpers
// =============================================================================

func makePendingUpdate(userID uuid.UUID, fieldName, proposedValue string) *domain.MedicalPendingUpdate {
	now := time.Now()
	docID := uuid.Must(uuid.NewV7())
	return &domain.MedicalPendingUpdate{
		ID:               uuid.Must(uuid.NewV7()),
		UserID:           userID,
		SourceType:       "ocr",
		SourceDocumentID: &docID,
		FieldName:        fieldName,
		CurrentValue:     new("valor anterior"),
		ProposedValue:    proposedValue,
		SuggestedAt:      now,
		ExpiresAt:        now.Add(30 * 24 * time.Hour),
		Status:           domain.PendingUpdatePending,
	}
}

func makeMedicalProfile(userID uuid.UUID) *domain.MedicalProfileV2 {
	return &domain.MedicalProfileV2{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		IsShared:  false,
		Data:      make(map[string]*domain.MedicalFieldValue),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestResolvePending_Accept(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	pu := makePendingUpdate(userID, "blood_type", "O+")

	pendingResolved := false
	profileUpdated := false

	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
				return pu, nil
			},
			resolveFn: func(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error {
				pendingResolved = true
				return nil
			},
		},
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfileV2) error {
				profileUpdated = true
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		PendingUpdateID: pu.ID.String(),
		UserID:          userID.String(),
		Action:          "accept",
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !pendingResolved {
		t.Error("el pending update debería haberse resuelto")
	}
	if !profileUpdated {
		t.Error("el perfil médico debería haberse actualizado")
	}
}

func TestResolvePending_Reject(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	pu := makePendingUpdate(userID, "allergies", "Penicilina, Sulfa")

	pendingResolved := false
	var resolvedStatus domain.MedicalPendingUpdateStatus

	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
				return pu, nil
			},
			resolveFn: func(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error {
				pendingResolved = true
				resolvedStatus = status
				return nil
			},
		},
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeMedicalProfile(userID), nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		PendingUpdateID: pu.ID.String(),
		UserID:          userID.String(),
		Action:          "reject",
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !pendingResolved {
		t.Error("el pending update debería haberse resuelto")
	}
	if resolvedStatus != domain.PendingUpdateRejected {
		t.Errorf("status = %s, se esperaba rejected", resolvedStatus)
	}
}

func TestResolvePending_Custom(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	pu := makePendingUpdate(userID, "allergies", "Penicilina, Sulfa")

	var updatedProfile *domain.MedicalProfileV2

	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
				return pu, nil
			},
		},
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfileV2) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	customValue := "Aspirina"
	cmd := Command{
		PendingUpdateID: pu.ID.String(),
		UserID:          userID.String(),
		Action:          "custom",
		CustomValue:     &customValue,
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if updatedProfile == nil {
		t.Fatal("el perfil debería haberse actualizado")
	}
}

func TestResolvePending_WrongOwnership(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	otherUserID := uuid.Must(uuid.NewV7())
	pu := makePendingUpdate(otherUserID, "blood_type", "O+")

	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
				return pu, nil
			},
		},
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeMedicalProfile(userID), nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		PendingUpdateID: pu.ID.String(),
		UserID:          userID.String(),
		Action:          "accept",
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error por ownership")
	}
	if !errors.Is(err, domain.ErrPendingUpdateNotFound) {
		t.Errorf("error = %v, se esperaba ErrPendingUpdateNotFound", err)
	}
}

func TestResolvePending_Expired(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	pu := makePendingUpdate(userID, "blood_type", "O+")
	// Simular expiración: setear ExpiresAt en el pasado
	past := time.Now().Add(-1 * time.Hour)
	pu.ExpiresAt = past

	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
				return pu, nil
			},
		},
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeMedicalProfile(userID), nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		PendingUpdateID: pu.ID.String(),
		UserID:          userID.String(),
		Action:          "accept",
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error por expiración")
	}
	if !errors.Is(err, domain.ErrPendingUpdateExpired) {
		t.Errorf("error = %v, se esperaba ErrPendingUpdateExpired", err)
	}
}

func TestResolvePending_InvalidAction(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	pu := makePendingUpdate(userID, "blood_type", "O+")

	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
				return pu, nil
			},
		},
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeMedicalProfile(userID), nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		PendingUpdateID: pu.ID.String(),
		UserID:          userID.String(),
		Action:          "invalid",
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error por acción inválida")
	}
	if !errors.Is(err, domain.ErrInvalidPendingAction) {
		t.Errorf("error = %v, se esperaba ErrInvalidPendingAction", err)
	}
}

func TestResolvePending_CustomWithoutValue(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	pu := makePendingUpdate(userID, "blood_type", "O+")

	uc := NewUseCase(UseCaseDeps{
		MedicalPendingRepo: &mockMedicalPendingRepo{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
				return pu, nil
			},
		},
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeMedicalProfile(userID), nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		PendingUpdateID: pu.ID.String(),
		UserID:          userID.String(),
		Action:          "custom",
		CustomValue:     nil, // falta custom_value
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error por falta de custom_value")
	}
	if !errors.Is(err, domain.ErrInvalidPendingAction) {
		t.Errorf("error = %v, se esperaba ErrInvalidPendingAction", err)
	}
}
