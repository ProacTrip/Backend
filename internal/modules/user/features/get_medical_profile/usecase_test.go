// RED — Test del usecase get_medical_profile.
// Verifica desencriptación transparente de campos médicos.
package get_medical_profile

import (
	"context"
	"encoding/base64"
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

type mockMedicalProfileRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfileV2, error)
	createFn      func(ctx context.Context, profile *domain.MedicalProfileV2) error
	updateFn      func(ctx context.Context, profile *domain.MedicalProfileV2) error
}

func (m *mockMedicalProfileRepo) Create(ctx context.Context, p *domain.MedicalProfileV2) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	return nil
}
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

type mockEncryptionService struct {
	encryptFn func(plaintext string) ([]byte, error)
	decryptFn func(ciphertext []byte) (string, error)
}

func (m *mockEncryptionService) Encrypt(plaintext string) ([]byte, error) {
	if m.encryptFn != nil {
		return m.encryptFn(plaintext)
	}
	return []byte("enc:" + plaintext), nil
}

func (m *mockEncryptionService) Decrypt(ciphertext []byte) (string, error) {
	if m.decryptFn != nil {
		return m.decryptFn(ciphertext)
	}
	// Simular desencriptación: ciphertext son bytes raw "enc:value"
	s := string(ciphertext)
	if strings.HasPrefix(s, "enc:") {
		return s[4:], nil
	}
	return s, nil
}

type mockMedicalPendingRepo struct {
	countPendingFn func(ctx context.Context, userID uuid.UUID) (int, error)
}

func (m *mockMedicalPendingRepo) Create(ctx context.Context, u *domain.MedicalPendingUpdate) error { return nil }
func (m *mockMedicalPendingRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.MedicalPendingUpdate, error) {
	return nil, nil
}
func (m *mockMedicalPendingRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
	return nil, nil
}
func (m *mockMedicalPendingRepo) Resolve(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error {
	return nil
}
func (m *mockMedicalPendingRepo) CountPending(ctx context.Context, userID uuid.UUID) (int, error) {
	if m.countPendingFn != nil {
		return m.countPendingFn(ctx, userID)
	}
	return 0, nil
}

// =============================================================================
// Helper: construir perfil médico con campos encriptados
// =============================================================================

func makeEncryptedValue(svc *mockEncryptionService, value string) string {
	encrypted, _ := svc.Encrypt(value)
	return base64.StdEncoding.EncodeToString(encrypted)
}

// =============================================================================
// Tests
// =============================================================================

func TestGetMedicalProfile_HappyPath(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	encSvc := &mockEncryptionService{}
	now := time.Now()

	// Campos en texto plano
	bloodValue := "A+"

	// Campos encriptados (simulados con base64)
	allergiesPlain := "Penicilina, Polen"
	medicationsPlain := "Loratadina 10mg"

	mp := &domain.MedicalProfileV2{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		IsShared: true,
		Data: map[string]*domain.MedicalFieldValue{
			"blood_type": {
				Value:     bloodValue,
				Source:    domain.MedicalSourceDetail{Type: "manual"},
				UpdatedAt: now,
			},
			"allergies_enc": {
				Value:     makeEncryptedValue(encSvc, allergiesPlain),
				Source:    domain.MedicalSourceDetail{Type: "manual"},
				UpdatedAt: now,
			},
			"medications_enc": {
				Value:     makeEncryptedValue(encSvc, medicationsPlain),
				Source:    domain.MedicalSourceDetail{Type: "ocr"},
				UpdatedAt: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return mp, nil
			},
		},
		EncryptionService: encSvc,
		MedicalPendingRepo: &mockMedicalPendingRepo{
			countPendingFn: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 2, nil
			},
		},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp == nil {
		t.Fatal("response no debería ser nil")
	}

	// Verificar blood_type (no encriptado)
	bt, ok := resp.Data["blood_type"]
	if !ok {
		t.Fatal("blood_type debería estar presente en la respuesta")
	}
	if bt.Value != "A+" {
		t.Errorf("blood_type = %s, se esperaba A+", bt.Value)
	}
	if bt.Source.Type != "manual" {
		t.Errorf("blood_type source = %s, se esperaba manual", bt.Source.Type)
	}

	// Verificar allergies (desencriptado, sin sufijo _enc)
	all, ok := resp.Data["allergies"]
	if !ok {
		t.Fatal("allergies debería estar presente en la respuesta (desencriptado, sin _enc)")
	}
	if all.Value != allergiesPlain {
		t.Errorf("allergies = %s, se esperaba %s", all.Value, allergiesPlain)
	}

	// Verificar medications (desencriptado)
	med, ok := resp.Data["medications"]
	if !ok {
		t.Fatal("medications debería estar presente en la respuesta")
	}
	if med.Value != medicationsPlain {
		t.Errorf("medications = %s, se esperaba %s", med.Value, medicationsPlain)
	}

	// Verificar is_shared
	if !resp.IsShared {
		t.Error("is_shared debería ser true")
	}

	// Verificar conflictos pendientes
	if !resp.HasPendingConflicts {
		t.Error("has_pending_conflicts debería ser true")
	}
	if resp.PendingConflictCount != 2 {
		t.Errorf("pending_conflict_count = %d, se esperaba 2", resp.PendingConflictCount)
	}
}

func TestGetMedicalProfile_EmptyProfile(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	now := time.Now()

	mp := &domain.MedicalProfileV2{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		IsShared:  false,
		Data:      make(map[string]*domain.MedicalFieldValue),
		CreatedAt: now,
		UpdatedAt: now,
	}

	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return mp, nil
			},
		},
		EncryptionService: &mockEncryptionService{},
		MedicalPendingRepo: &mockMedicalPendingRepo{
			countPendingFn: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 0, nil
			},
		},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.Data) != 0 {
		t.Errorf("data debería estar vacío, tiene %d campos", len(resp.Data))
	}
	if resp.HasPendingConflicts {
		t.Error("has_pending_conflicts debería ser false")
	}
	if resp.PendingConflictCount != 0 {
		t.Errorf("pending_conflict_count = %d, se esperaba 0", resp.PendingConflictCount)
	}
}

func TestGetMedicalProfile_MedicalProfileNotFound(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return nil, domain.ErrMedicalProfileNotFound
			},
		},
		EncryptionService:    &mockEncryptionService{},
		MedicalPendingRepo:   &mockMedicalPendingRepo{},
	})

	cmd := Command{UserID: uuid.Must(uuid.NewV7()).String()}
	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error ErrMedicalProfileNotFound")
	}
	if !errors.Is(err, domain.ErrMedicalProfileNotFound) {
		t.Errorf("error = %v, se esperaba ErrMedicalProfileNotFound", err)
	}
}

func TestGetMedicalProfile_DecryptionError(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	now := time.Now()

	// Perfil con dato encriptado corrupto (no es base64 válido)
	mp := &domain.MedicalProfileV2{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		IsShared: false,
		Data: map[string]*domain.MedicalFieldValue{
			"allergies_enc": {
				Value:     "esto-no-es-base64!!!",
				Source:    domain.MedicalSourceDetail{Type: "manual"},
				UpdatedAt: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return mp, nil
			},
		},
		EncryptionService: &mockEncryptionService{},
		MedicalPendingRepo: &mockMedicalPendingRepo{},
	})

	cmd := Command{UserID: userID.String()}
	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error de desencriptación")
	}
	if !errors.Is(err, domain.ErrDecryptionError) {
		t.Errorf("error = %v, se esperaba ErrDecryptionError", err)
	}
}

func TestGetMedicalProfile_NoPendingConflicts(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	now := time.Now()

	mp := &domain.MedicalProfileV2{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		IsShared: false,
		Data: map[string]*domain.MedicalFieldValue{
			"blood_type": {
				Value:     "O+",
				Source:    domain.MedicalSourceDetail{Type: "manual"},
				UpdatedAt: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return mp, nil
			},
		},
		EncryptionService: &mockEncryptionService{},
		MedicalPendingRepo: &mockMedicalPendingRepo{
			countPendingFn: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 0, nil
			},
		},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.HasPendingConflicts {
		t.Error("has_pending_conflicts debería ser false")
	}
	if resp.PendingConflictCount != 0 {
		t.Errorf("pending_conflict_count = %d, se esperaba 0", resp.PendingConflictCount)
	}
}
