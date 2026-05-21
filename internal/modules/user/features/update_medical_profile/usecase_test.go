// RED — Test del usecase update_medical_profile.
// Verifica encriptación transparente de campos médicos al guardar.
package update_medical_profile

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
	s := string(ciphertext)
	if strings.HasPrefix(s, "enc:") {
		return s[4:], nil
	}
	return s, nil
}

// =============================================================================
// Helper: construir perfil médico de base
// =============================================================================

func makeBaseMedicalProfile(userID uuid.UUID) *domain.MedicalProfileV2 {
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

func TestUpdateMedicalProfile_HappyPath(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	encSvc := &mockEncryptionService{}

	allergies := "Penicilina"
	bloodType := "A+"
	isShared := true

	var updatedProfile *domain.MedicalProfileV2
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfileV2) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: encSvc,
	})

	cmd := Command{
		UserID:    userID.String(),
		Allergies: &allergies,
		BloodType: &bloodType,
		IsShared:  &isShared,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// Verificar applied_fields
	if len(resp.AppliedFields) != 3 {
		t.Errorf("applied_fields = %d campos, se esperaban 3", len(resp.AppliedFields))
	}

	// Verificar que blood_type se guardó sin _enc
	btField, ok := updatedProfile.Data["blood_type"]
	if !ok {
		t.Fatal("blood_type debería estar en el perfil")
	}
	if btField.Value != "A+" {
		t.Errorf("blood_type value = %s, se esperaba A+", btField.Value)
	}
	if btField.Source.Type != "manual" {
		t.Errorf("blood_type source = %s, se esperaba manual", btField.Source.Type)
	}

	// Verificar que allergies se guardó con _enc y encriptado
	allField, ok := updatedProfile.Data["allergies_enc"]
	if !ok {
		t.Fatal("allergies_enc debería estar en el perfil")
	}

	// Decodificar y verificar que contiene el prefijo de encriptación simulado
	decoded, err := base64.StdEncoding.DecodeString(allField.Value)
	if err != nil {
		t.Fatalf("error decodificando base64: %v", err)
	}
	if !strings.HasPrefix(string(decoded), "enc:") {
		t.Error("allergies debería estar encriptado")
	}

	// Verificar is_shared
	if !updatedProfile.IsShared {
		t.Error("is_shared debería ser true")
	}
}

func TestUpdateMedicalProfile_SkipNilFields(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	// Solo actualizar blood_type, el resto nil
	bloodType := "O-"

	var updatedProfile *domain.MedicalProfileV2
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfileV2) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:    userID.String(),
		BloodType: &bloodType,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.AppliedFields) != 1 {
		t.Errorf("applied_fields = %d, se esperaba 1 (solo blood_type)", len(resp.AppliedFields))
	}
	if len(updatedProfile.Data) != 1 {
		t.Errorf("Data size = %d, se esperaba 1", len(updatedProfile.Data))
	}
}

func TestUpdateMedicalProfile_InvalidBloodType(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	invalidBT := "XYZ"

	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeBaseMedicalProfile(userID), nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:    userID.String(),
		BloodType: &invalidBT,
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error de tipo de sangre inválido")
	}
	if !errors.Is(err, domain.ErrInvalidBloodType) {
		t.Errorf("error = %v, se esperaba ErrInvalidBloodType", err)
	}
}

func TestUpdateMedicalProfile_ValidBloodTypes(t *testing.T) {
	validTypes := []string{"A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-"}

	userID := uuid.Must(uuid.NewV7())

	for _, bt := range validTypes {
		t.Run(bt, func(t *testing.T) {
			uc := NewUseCase(UseCaseDeps{
				MedicalProfileRepo: &mockMedicalProfileRepo{
					getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
						return makeBaseMedicalProfile(userID), nil
					},
					updateFn: func(ctx context.Context, p *domain.MedicalProfileV2) error {
						return nil
					},
				},
				EncryptionService: &mockEncryptionService{},
			})

			cmd := Command{
				UserID:    userID.String(),
				BloodType: &bt,
			}

			_, err := uc.Execute(t.Context(), cmd)
			if err != nil {
				t.Errorf("no se esperaba error para %s: %v", bt, err)
			}
		})
	}
}

func TestUpdateMedicalProfile_EncryptsMultipleFields(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	allergies := "Penicilina"
	medications := "Loratadina"
	conditions := "Asma"
	vaccinations := "COVID-19"
	emergencyContact := "María +54911"
	insuranceInfo := "Póliza 12345"

	var updatedProfile *domain.MedicalProfileV2
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfileV2) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:           userID.String(),
		Allergies:        &allergies,
		Medications:      &medications,
		Conditions:       &conditions,
		Vaccinations:     &vaccinations,
		EmergencyContact: &emergencyContact,
		InsuranceInfo:    &insuranceInfo,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.AppliedFields) != 6 {
		t.Errorf("applied_fields = %d, se esperaban 6", len(resp.AppliedFields))
	}

	// Todos deben tener sufijo _enc
	encFields := []string{"allergies_enc", "medications_enc", "conditions_enc",
		"vaccinations_enc", "emergency_contact_enc", "insurance_info_enc"}
	for _, f := range encFields {
		if _, ok := updatedProfile.Data[f]; !ok {
			t.Errorf("%s debería estar presente en el perfil con sufijo _enc", f)
		}
	}
}

func TestUpdateMedicalProfile_ProfileNotFound(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfileV2, error) {
				return nil, domain.ErrMedicalProfileNotFound
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{UserID: uuid.Must(uuid.NewV7()).String()}
	_, err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrMedicalProfileNotFound) {
		t.Errorf("error = %v, se esperaba ErrMedicalProfileNotFound", err)
	}
}
