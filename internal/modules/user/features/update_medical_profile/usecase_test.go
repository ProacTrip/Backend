// RED — Test del usecase update_medical_profile.
// Verifica encriptación transparente de campos médicos al guardar.
package update_medical_profile

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error)
	updateFn      func(ctx context.Context, profile *domain.MedicalProfile) error
}

func (m *mockMedicalProfileRepo) Create(ctx context.Context, p *domain.MedicalProfile) error { return nil }
func (m *mockMedicalProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockMedicalProfileRepo) Update(ctx context.Context, p *domain.MedicalProfile) error {
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

func makeBaseMedicalProfile(userID uuid.UUID) *domain.MedicalProfile {
	return &domain.MedicalProfile{
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

	allergies := []string{"Penicilina"}
	bloodType := "A+"
	isShared := true

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
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

	// Verificar que allergies se guardó con _enc y encriptado (ahora es JSON marshalizado)
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

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
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
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
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
					getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
						return makeBaseMedicalProfile(userID), nil
					},
					updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
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

	allergies := []string{"Penicilina"}
	medications := []domain.Medication{{Name: "Loratadina", Dosage: "10mg", Frequency: "diaria", Duration: "crónico", Status: "active"}}
	conditions := []string{"Asma"}
	vaccinations := []domain.Vaccination{{Name: "COVID-19", DosesReceived: 2, Status: "completed"}}
	emergencyContact := domain.EmergencyContact{Name: "María", Phone: "+54911"}
	insuranceInfo := domain.InsuranceInfo{Company: "Póliza 12345", PolicyNumber: "12345"}

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
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
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
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

// =============================================================================
// T-2.1: Command con campos tipados (ya no *string)
// =============================================================================

func TestUpdateMedicalProfile_TypedAllergiesCommand(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	allergies := []string{"Penicilina", "Polen"}

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:    userID.String(),
		Allergies: &allergies,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.AppliedFields) != 1 {
		t.Fatalf("applied_fields = %d, se esperaba 1", len(resp.AppliedFields))
	}

	// Verificar que el campo encriptado contiene el JSON marshalizado de allergies
	allField, ok := updatedProfile.Data["allergies_enc"]
	if !ok {
		t.Fatal("allergies_enc debería estar presente")
	}

	decoded, err := base64.StdEncoding.DecodeString(allField.Value)
	if err != nil {
		t.Fatalf("error decodificando base64: %v", err)
	}

	// Desencriptar con el mock (quita prefijo "enc:")
	plaintext := string(decoded)
	if !strings.HasPrefix(plaintext, "enc:") {
		t.Fatal("debería estar encriptado con prefijo 'enc:'")
	}
	jsonStr := plaintext[4:] // quitar prefijo "enc:"

	var decodedAllergies []string
	if err := json.Unmarshal([]byte(jsonStr), &decodedAllergies); err != nil {
		t.Fatalf("error unmarshal JSON: %v — contenido: %q", err, jsonStr)
	}
	if len(decodedAllergies) != 2 {
		t.Errorf("len(allergies) = %d, se esperaba 2", len(decodedAllergies))
	}
	if decodedAllergies[0] != "Penicilina" || decodedAllergies[1] != "Polen" {
		t.Errorf("allergies = %v, se esperaba [Penicilina Polen]", decodedAllergies)
	}
}

func TestUpdateMedicalProfile_TypedMedicationsCommand(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	medications := []domain.Medication{
		{Name: "Ibuprofeno", Dosage: "600mg", Frequency: "Cada 8 horas", Duration: "5 días", Status: "active"},
		{Name: "Omeprazol", Dosage: "20mg", Frequency: "Cada 24 horas", Duration: "crónico", Status: "active"},
	}

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:      userID.String(),
		Medications: &medications,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.AppliedFields) != 1 {
		t.Fatalf("applied_fields = %d, se esperaba 1", len(resp.AppliedFields))
	}

	medField, ok := updatedProfile.Data["medications_enc"]
	if !ok {
		t.Fatal("medications_enc debería estar presente")
	}

	decoded, err := base64.StdEncoding.DecodeString(medField.Value)
	if err != nil {
		t.Fatalf("error decodificando base64: %v", err)
	}

	plaintext := string(decoded)
	if !strings.HasPrefix(plaintext, "enc:") {
		t.Fatal("debería estar encriptado")
	}
	jsonStr := plaintext[4:]

	var decodedMeds []domain.Medication
	if err := json.Unmarshal([]byte(jsonStr), &decodedMeds); err != nil {
		t.Fatalf("error unmarshal medications JSON: %v — contenido: %q", err, jsonStr)
	}
	if len(decodedMeds) != 2 {
		t.Errorf("len(medications) = %d, se esperaba 2", len(decodedMeds))
	}
	if decodedMeds[0].Name != "Ibuprofeno" {
		t.Errorf("med[0].Name = %s, se esperaba Ibuprofeno", decodedMeds[0].Name)
	}
}

func TestUpdateMedicalProfile_TypedVaccinationsCommand(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	vaccinations := []domain.Vaccination{
		{Name: "COVID-19", DosesReceived: 3, Status: "completed"},
	}

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:       userID.String(),
		Vaccinations: &vaccinations,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.AppliedFields) != 1 {
		t.Fatalf("applied_fields = %d, se esperaba 1", len(resp.AppliedFields))
	}

	vacField, ok := updatedProfile.Data["vaccinations_enc"]
	if !ok {
		t.Fatal("vaccinations_enc debería estar presente")
	}
	decoded, _ := base64.StdEncoding.DecodeString(vacField.Value)
	plaintext := string(decoded)
	jsonStr := plaintext[4:]

	var decodedVacs []domain.Vaccination
	if err := json.Unmarshal([]byte(jsonStr), &decodedVacs); err != nil {
		t.Fatalf("error unmarshal vaccinations JSON: %v", err)
	}
	if len(decodedVacs) != 1 || decodedVacs[0].Name != "COVID-19" {
		t.Error("vaccination data mismatch")
	}
}

func TestUpdateMedicalProfile_TypedEmergencyContactCommand(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	ec := domain.EmergencyContact{Name: "María García", Phone: "+5491123456790"}

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:           userID.String(),
		EmergencyContact: &ec,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.AppliedFields) != 1 {
		t.Fatalf("applied_fields = %d, se esperaba 1", len(resp.AppliedFields))
	}

	ecField, ok := updatedProfile.Data["emergency_contact_enc"]
	if !ok {
		t.Fatal("emergency_contact_enc debería estar presente")
	}
	decoded, _ := base64.StdEncoding.DecodeString(ecField.Value)
	plaintext := string(decoded)
	jsonStr := plaintext[4:]

	var decodedEC domain.EmergencyContact
	if err := json.Unmarshal([]byte(jsonStr), &decodedEC); err != nil {
		t.Fatalf("error unmarshal emergency_contact JSON: %v", err)
	}
	if decodedEC.Name != "María García" {
		t.Errorf("Name = %s, se esperaba María García", decodedEC.Name)
	}
}

func TestUpdateMedicalProfile_TypedInsuranceInfoCommand(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	insurance := domain.InsuranceInfo{Company: "ASSA", PolicyNumber: "12345"}

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:        userID.String(),
		InsuranceInfo: &insurance,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.AppliedFields) != 1 {
		t.Fatalf("applied_fields = %d, se esperaba 1", len(resp.AppliedFields))
	}

	insField, ok := updatedProfile.Data["insurance_info_enc"]
	if !ok {
		t.Fatal("insurance_info_enc debería estar presente")
	}
	decoded, _ := base64.StdEncoding.DecodeString(insField.Value)
	plaintext := string(decoded)
	jsonStr := plaintext[4:]

	var decodedIns domain.InsuranceInfo
	if err := json.Unmarshal([]byte(jsonStr), &decodedIns); err != nil {
		t.Fatalf("error unmarshal insurance_info JSON: %v", err)
	}
	if decodedIns.Company != "ASSA" || decodedIns.PolicyNumber != "12345" {
		t.Error("insurance_info mismatch")
	}
}

func TestUpdateMedicalProfile_TypedConditionsCommand(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	conditions := []string{"Asma leve", "Diabetes tipo 2"}

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:     userID.String(),
		Conditions: &conditions,
	}

	_, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	condField, ok := updatedProfile.Data["conditions_enc"]
	if !ok {
		t.Fatal("conditions_enc debería estar presente")
	}
	decoded, _ := base64.StdEncoding.DecodeString(condField.Value)
	plaintext := string(decoded)
	jsonStr := plaintext[4:]

	var decodedCond []string
	if err := json.Unmarshal([]byte(jsonStr), &decodedCond); err != nil {
		t.Fatalf("error unmarshal conditions JSON: %v", err)
	}
	if len(decodedCond) != 2 || decodedCond[0] != "Asma leve" {
		t.Error("conditions data mismatch")
	}
}

// =============================================================================
// T-2.2: Marshal round-trip — typed value → json.Marshal → encrypt → base64
// =============================================================================

func TestUpdateMedicalProfile_MarshalRoundTrip_AllTypes(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	allergies := []string{"Penicilina"}
	conditions := []string{"Asma"}
	medications := []domain.Medication{
		{Name: "Loratadina", Dosage: "10mg", Frequency: "diaria", Duration: "crónico", Status: "active"},
	}
	vaccinations := []domain.Vaccination{
		{Name: "COVID-19", DosesReceived: 2, Status: "completed"},
	}
	ec := domain.EmergencyContact{Name: "Juan", Phone: "+5491123456789"}
	insurance := domain.InsuranceInfo{Company: "OSDE", PolicyNumber: "999"}

	var updatedProfile *domain.MedicalProfile
	uc := NewUseCase(UseCaseDeps{
		MedicalProfileRepo: &mockMedicalProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.MedicalProfile, error) {
				return makeBaseMedicalProfile(userID), nil
			},
			updateFn: func(ctx context.Context, p *domain.MedicalProfile) error {
				updatedProfile = p
				return nil
			},
		},
		EncryptionService: &mockEncryptionService{},
	})

	cmd := Command{
		UserID:           userID.String(),
		Allergies:        &allergies,
		Conditions:       &conditions,
		Medications:      &medications,
		Vaccinations:     &vaccinations,
		EmergencyContact: &ec,
		InsuranceInfo:    &insurance,
	}

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(resp.AppliedFields) != 6 {
		t.Fatalf("applied_fields = %d, se esperaban 6", len(resp.AppliedFields))
	}

	// Verificar cada campo: round-trip marshal + encrypt
	tests := []struct {
		encKey     string
		targetType string // "[]string", "[]domain.Medication", etc.
	}{
		{"allergies_enc", "[]string"},
		{"conditions_enc", "[]string"},
		{"medications_enc", "[]domain.Medication"},
		{"vaccinations_enc", "[]domain.Vaccination"},
		{"emergency_contact_enc", "domain.EmergencyContact"},
		{"insurance_info_enc", "domain.InsuranceInfo"},
	}

	for _, tc := range tests {
		t.Run(tc.encKey, func(t *testing.T) {
			field, ok := updatedProfile.Data[tc.encKey]
			if !ok {
				t.Fatalf("%s no encontrado", tc.encKey)
			}
			decoded, err := base64.StdEncoding.DecodeString(field.Value)
			if err != nil {
				t.Fatalf("base64 decode falló: %v", err)
			}
			plaintext := string(decoded)
			if !strings.HasPrefix(plaintext, "enc:") {
				t.Fatal("no encriptado")
			}
			// Verificar que el JSON es válido (sin panics)
			var any interface{}
			jsonStr := plaintext[4:]
			if err := json.Unmarshal([]byte(jsonStr), &any); err != nil {
				t.Fatalf("JSON inválido para %s: %v — contenido: %q", tc.encKey, err, jsonStr)
			}
		})
	}
}
