package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	searchdomain "github.com/ProacTrip/Backend/internal/modules/search/domain"
	userdomain "github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// RED: Tasks 1.2-1.4 — UserHealthAdapter
// These tests reference types/functions that do NOT exist yet.
// =============================================================================

// =============================================================================
// Mock repositories for testing
// =============================================================================

type mockProfileRepo struct {
	profile *userdomain.UserProfile
	err     error
}

func (m *mockProfileRepo) Create(ctx context.Context, profile *userdomain.UserProfile) error { return nil }
func (m *mockProfileRepo) UpsertProfile(ctx context.Context, profile *userdomain.UserProfile) error { return nil }
func (m *mockProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*userdomain.UserProfile, error) {
	return m.profile, m.err
}
func (m *mockProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*userdomain.UserProfile, error) { return nil, nil }
func (m *mockProfileRepo) Update(ctx context.Context, profile *userdomain.UserProfile) error { return nil }
func (m *mockProfileRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, language, currency string) error { return nil }
func (m *mockProfileRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error { return nil }
func (m *mockProfileRepo) UpdatePreferences(ctx context.Context, userID uuid.UUID, language, currency string) error { return nil }

type mockTravelRepo struct {
	prefs *userdomain.TravelPreferences
	err    error
}

func (m *mockTravelRepo) Create(ctx context.Context, prefs *userdomain.TravelPreferences) error { return nil }
func (m *mockTravelRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*userdomain.TravelPreferences, error) {
	return m.prefs, m.err
}
func (m *mockTravelRepo) Update(ctx context.Context, prefs *userdomain.TravelPreferences) error { return nil }

type mockMedicalRepo struct {
	profile *userdomain.MedicalProfile
	err     error
}

func (m *mockMedicalRepo) Create(ctx context.Context, profile *userdomain.MedicalProfile) error { return nil }
func (m *mockMedicalRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*userdomain.MedicalProfile, error) {
	return m.profile, m.err
}
func (m *mockMedicalRepo) Update(ctx context.Context, profile *userdomain.MedicalProfile) error { return nil }

type mockDocumentRepo struct {
	docs []*userdomain.UserDocument
	err  error
}

func (m *mockDocumentRepo) Create(ctx context.Context, doc *userdomain.UserDocument) error { return nil }
func (m *mockDocumentRepo) GetByID(ctx context.Context, id uuid.UUID) (*userdomain.UserDocument, error) { return nil, nil }
func (m *mockDocumentRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*userdomain.UserDocument, error) {
	return m.docs, m.err
}
func (m *mockDocumentRepo) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) { return 0, nil }
func (m *mockDocumentRepo) Update(ctx context.Context, doc *userdomain.UserDocument) error { return nil }
func (m *mockDocumentRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockDocumentRepo) GetTypes(ctx context.Context) ([]userdomain.DocumentType, error) { return nil, nil }

// =============================================================================
// TESTS
// =============================================================================

func TestUserHealthAdapter_ImplementsPort(t *testing.T) {
	// Compile-time check would be in the adapter file:
	// var _ searchdomain.UserHealthPort = (*UserHealthAdapter)(nil)

	adapter := NewUserHealthAdapter(nil, nil, nil, nil)
	if adapter == nil {
		t.Fatal("NewUserHealthAdapter returned nil")
	}

	// Runtime check: the adapter should satisfy the interface
	var port searchdomain.UserHealthPort = adapter
	if port == nil {
		t.Fatal("adapter does not satisfy UserHealthPort")
	}
	t.Log("UserHealthAdapter satisfies UserHealthPort interface")
}

func TestGetMedicalContext_EmptyWhenNoProfile(t *testing.T) {
	adapter := NewUserHealthAdapter(nil, nil, nil, nil)

	ctx := context.Background()
	result, err := adapter.GetMedicalContext(ctx, uuid.Must(uuid.NewV7()).String())
	if err != nil {
		t.Fatalf("GetMedicalContext with nil repo should not error: %v", err)
	}
	if result == nil {
		t.Fatal("GetMedicalContext should return non-nil MedicalAIContext even when profile is missing")
	}
	// All fields should be empty/zero
	if len(result.Allergies) != 0 {
		t.Errorf("expected empty Allergies, got %v", result.Allergies)
	}
	if result.BloodType != "" {
		t.Errorf("expected empty BloodType, got %q", result.BloodType)
	}
}

func TestGetMedicalContext_MapsMedicalFields(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	profile := &userdomain.MedicalProfile{
		ID:     uuid.Must(uuid.NewV7()),
		UserID: userID,
		Data: map[string]*userdomain.MedicalFieldValue{
			"alergias":    {Value: "Maní, Polen"},
			"condiciones": {Value: "Asma leve"},
			"medicamentos": {Value: "Ibuprofeno"},
			"vacunas":      {Value: "Fiebre amarilla, Hepatitis B"},
			"tipo_sangre":  {Value: "O+"},
			"nota_personal": {Value: "Evitar sol directo"},
		},
	}

	adapter := NewUserHealthAdapter(nil, nil, &mockMedicalRepo{profile: profile}, nil)

	ctx := context.Background()
	result, err := adapter.GetMedicalContext(ctx, userID.String())
	if err != nil {
		t.Fatalf("GetMedicalContext: %v", err)
	}

	if len(result.Allergies) != 1 || result.Allergies[0] != "Maní, Polen" {
		t.Errorf("Allergies = %v, want [Maní, Polen]", result.Allergies)
	}
	if len(result.Conditions) != 1 || result.Conditions[0] != "Asma leve" {
		t.Errorf("Conditions = %v, want [Asma leve]", result.Conditions)
	}
	if len(result.Medications) != 1 || result.Medications[0] != "Ibuprofeno" {
		t.Errorf("Medications = %v, want [Ibuprofeno]", result.Medications)
	}
	if len(result.Vaccinations) != 1 || result.Vaccinations[0] != "Fiebre amarilla, Hepatitis B" {
		t.Errorf("Vaccinations = %v, want [Fiebre amarilla, Hepatitis B]", result.Vaccinations)
	}
	if result.BloodType != "O+" {
		t.Errorf("BloodType = %q, want O+", result.BloodType)
	}
	if extra, ok := result.Extra["nota_personal"]; !ok || extra != "Evitar sol directo" {
		t.Errorf("Extra[nota_personal] = %v, want 'Evitar sol directo'", extra)
	}
}

func TestGetMedicalContext_CaseInsensitiveMapping(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	profile := &userdomain.MedicalProfile{
		ID:     uuid.Must(uuid.NewV7()),
		UserID: userID,
		Data: map[string]*userdomain.MedicalFieldValue{
			"ALLERGIES":   {Value: "Latex"},
			"Alergias":    {Value: "Gluten"},
		},
	}

	adapter := NewUserHealthAdapter(nil, nil, &mockMedicalRepo{profile: profile}, nil)

	ctx := context.Background()
	result, err := adapter.GetMedicalContext(ctx, userID.String())
	if err != nil {
		t.Fatalf("GetMedicalContext: %v", err)
	}

	// Both variations of allergies/asthma should be collected
	totalAllergies := len(result.Allergies)
	if totalAllergies != 2 {
		t.Errorf("expected 2 allergies (case-insensitive), got %d: %v", totalAllergies, result.Allergies)
	}
}

func TestGetTravelPreferences_MapsOneToOne(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mealPref := "vegetariano"
	maxDuration := 90
	prefs := &userdomain.TravelPreferences{
		ID:             uuid.Must(uuid.NewV7()),
		UserID:         userID,
		PreferredClass: userdomain.CabinClassBusiness,
		SeatPreference: new(userdomain.SeatPreference),
		MealPreference: &mealPref,
		SpecialAssistance: []string{"silla de ruedas"},
		AvoidLayovers:     true,
		MaxLayoverDuration: &maxDuration,
	}
	*prefs.SeatPreference = userdomain.SeatWindow

	adapter := NewUserHealthAdapter(nil, &mockTravelRepo{prefs: prefs}, nil, nil)

	ctx := context.Background()
	result, err := adapter.GetTravelPreferences(ctx, userID.String())
	if err != nil {
		t.Fatalf("GetTravelPreferences: %v", err)
	}

	if result.PreferredClass != "business" {
		t.Errorf("PreferredClass = %q, want 'business'", result.PreferredClass)
	}
	if result.SeatPreference != "window" {
		t.Errorf("SeatPreference = %q, want 'window'", result.SeatPreference)
	}
	if result.MealPreference != "vegetariano" {
		t.Errorf("MealPreference = %q, want 'vegetariano'", result.MealPreference)
	}
	if !result.AvoidLayovers {
		t.Error("AvoidLayovers should be true")
	}
	if result.MaxLayoverDuration != 90 {
		t.Errorf("MaxLayoverDuration = %d, want 90", result.MaxLayoverDuration)
	}
}

func TestGetTravelPreferences_EmptyWhenNoPrefs(t *testing.T) {
	adapter := NewUserHealthAdapter(nil, nil, nil, nil)

	ctx := context.Background()
	result, err := adapter.GetTravelPreferences(ctx, uuid.Must(uuid.NewV7()).String())
	if err != nil {
		t.Fatalf("GetTravelPreferences with nil repo should not error: %v", err)
	}
	if result == nil {
		t.Fatal("GetTravelPreferences should return non-nil TravelAIContext")
	}
}

func TestGetDocumentContext_FiltersPassportAndVisa(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	passportTypeID := uuid.Must(uuid.NewV7())
	visaTypeID := uuid.Must(uuid.NewV7())
	insuranceTypeID := uuid.Must(uuid.NewV7())

	// Passport with ExtractedData
	passportData := json.RawMessage(`{"document_number":"A12345678","issuing_country":"PER","nationality":"PER","valid_until":"2030-01-14","name":"Juan Pérez"}`)
	passportDoc := &userdomain.UserDocument{
		ID:             uuid.Must(uuid.NewV7()),
		UserID:         userID,
		DocumentTypeID: &passportTypeID,
		FileName:       "passport.pdf",
		StorageKey:     "docs/passport.pdf",
		ExtractedData:  passportData,
	}

	// Visa with minimal data
	visaData := json.RawMessage(`{"document_number":"V98765432","issuing_country":"USA","valid_until":"2028-06-30"}`)
	visaDoc := &userdomain.UserDocument{
		ID:             uuid.Must(uuid.NewV7()),
		UserID:         userID,
		DocumentTypeID: &visaTypeID,
		FileName:       "visa.pdf",
		StorageKey:     "docs/visa.pdf",
		ExtractedData:  visaData,
	}

	// Insurance — should be filtered OUT
	insuranceData := json.RawMessage(`{"policy_number":"INS-123"}`)
	insuranceDoc := &userdomain.UserDocument{
		ID:             uuid.Must(uuid.NewV7()),
		UserID:         userID,
		DocumentTypeID: &insuranceTypeID,
		FileName:       "insurance.pdf",
		StorageKey:     "docs/insurance.pdf",
		ExtractedData:  insuranceData,
	}

	// Our mock needs to know which types map to which codes.
	// The adapter filters by DocumentType.Code, which is on the type, not the document.
	// We need to mock GetTypes too, but since this is a unit test we use a custom mock.

	mockDocs := &mockDocumentRepoWithTypes{
		docs: []*userdomain.UserDocument{passportDoc, visaDoc, insuranceDoc},
		types: []userdomain.DocumentType{
			{ID: passportTypeID, Code: "passport", Name: "Pasaporte"},
			{ID: visaTypeID, Code: "visa", Name: "Visa"},
			{ID: insuranceTypeID, Code: "insurance", Name: "Seguro"},
		},
	}

	adapter := NewUserHealthAdapter(nil, nil, nil, mockDocs)

	ctx := context.Background()
	results, err := adapter.GetDocumentContext(ctx, userID.String())
	if err != nil {
		t.Fatalf("GetDocumentContext: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 documents (passport + visa), got %d", len(results))
	}

	// First doc should be passport
	passport := results[0]
	if passport.Type != "passport" {
		t.Errorf("Type = %q, want 'passport'", passport.Type)
	}
	if passport.Number != "A12345678" {
		t.Errorf("Number = %q, want 'A12345678'", passport.Number)
	}
	if passport.IssuingCountry != "PER" {
		t.Errorf("IssuingCountry = %q, want 'PER'", passport.IssuingCountry)
	}
	if passport.Nationality != "PER" {
		t.Errorf("Nationality = %q, want 'PER'", passport.Nationality)
	}
	if passport.ValidUntil != "2030-01-14" {
		t.Errorf("ValidUntil = %q, want '2030-01-14'", passport.ValidUntil)
	}
	if extra, ok := passport.Extra["name"]; !ok || extra != "Juan Pérez" {
		t.Errorf("Extra[name] = %v, want 'Juan Pérez'", extra)
	}

	// Second doc should be visa
	visa := results[1]
	if visa.Type != "visa" {
		t.Errorf("Type = %q, want 'visa'", visa.Type)
	}
}

func TestGetDocumentContext_EmptyWhenNoDocs(t *testing.T) {
	adapter := NewUserHealthAdapter(nil, nil, nil, nil)

	ctx := context.Background()
	results, err := adapter.GetDocumentContext(ctx, uuid.Must(uuid.NewV7()).String())
	if err != nil {
		t.Fatalf("GetDocumentContext with nil repo should not error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 documents, got %d", len(results))
	}
}

func TestGetDocumentContext_RepoError(t *testing.T) {
	mockDocs := &mockDocumentRepo{err: errors.New("db connection error")}
	adapter := NewUserHealthAdapter(nil, nil, nil, mockDocs)

	ctx := context.Background()
	_, err := adapter.GetDocumentContext(ctx, uuid.Must(uuid.NewV7()).String())

	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// =============================================================================
// mockDocumentRepoWithTypes extends mockDocumentRepo with GetTypes support
// =============================================================================

type mockDocumentRepoWithTypes struct {
	docs  []*userdomain.UserDocument
	types []userdomain.DocumentType
	err   error
}

func (m *mockDocumentRepoWithTypes) Create(ctx context.Context, doc *userdomain.UserDocument) error { return nil }
func (m *mockDocumentRepoWithTypes) GetByID(ctx context.Context, id uuid.UUID) (*userdomain.UserDocument, error) {
	return nil, nil
}
func (m *mockDocumentRepoWithTypes) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*userdomain.UserDocument, error) {
	return m.docs, m.err
}
func (m *mockDocumentRepoWithTypes) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) { return 0, nil }
func (m *mockDocumentRepoWithTypes) Update(ctx context.Context, doc *userdomain.UserDocument) error { return nil }
func (m *mockDocumentRepoWithTypes) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockDocumentRepoWithTypes) GetTypes(ctx context.Context) ([]userdomain.DocumentType, error) {
	return m.types, nil
}
