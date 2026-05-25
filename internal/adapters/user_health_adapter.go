// UserHealthAdapter adapta los repositorios del módulo user al puerto
// search/domain.UserHealthPort para inyectar contexto médico y de viaje
// en el system prompt de la IA.
package adapters

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	searchdomain "github.com/ProacTrip/Backend/internal/modules/search/domain"
	userdomain "github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// UserHealthAdapter implements searchdomain.UserHealthPort by delegating to
// user module repositories (ProfileRepository, TravelPrefsRepository,
// MedicalProfileRepository, DocumentRepository).
type UserHealthAdapter struct {
	profileRepo  userdomain.ProfileRepository
	travelRepo   userdomain.TravelPrefsRepository
	medicalRepo  userdomain.MedicalProfileRepository
	documentRepo userdomain.DocumentRepository
}

// Compile-time guard: ensure UserHealthAdapter satisfies searchdomain.UserHealthPort.
var _ searchdomain.UserHealthPort = (*UserHealthAdapter)(nil)

// NewUserHealthAdapter creates a new adapter backed by the given user module repos.
func NewUserHealthAdapter(
	profileRepo userdomain.ProfileRepository,
	travelRepo userdomain.TravelPrefsRepository,
	medicalRepo userdomain.MedicalProfileRepository,
	documentRepo userdomain.DocumentRepository,
) *UserHealthAdapter {
	return &UserHealthAdapter{
		profileRepo:  profileRepo,
		travelRepo:   travelRepo,
		medicalRepo:  medicalRepo,
		documentRepo: documentRepo,
	}
}

// =============================================================================
// GetMedicalContext — perfil médico → MedicalAIContext
// =============================================================================

// medicalFieldMapping maps known medical field names (case-insensitive, Spanish + English)
// to their target category in MedicalAIContext.
var medicalFieldMapping = map[string]string{
	"alergias":    "allergies",
	"allergies":   "allergies",
	"condiciones": "conditions",
	"conditions":  "conditions",
	"medicamentos": "medications",
	"medications": "medications",
	"vacunas":     "vaccinations",
	"vaccinations": "vaccinations",
	"tipo_sangre": "blood_type",
	"blood_type":  "blood_type",
	"grupo_sanguineo": "blood_type",
}

// GetMedicalContext reads the user's medical profile and extracts only the
// .Value field from each MedicalFieldValue, mapping known keys to named fields
// and preserving unknown keys in Extra.
func (a *UserHealthAdapter) GetMedicalContext(ctx context.Context, userID string) (*searchdomain.MedicalAIContext, error) {
	ctxResult := &searchdomain.MedicalAIContext{}

	if a.medicalRepo == nil {
		return ctxResult, nil
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return ctxResult, nil // invalid userID → empty context
	}

	profile, err := a.medicalRepo.GetByUserID(ctx, uid)
	if err != nil || profile == nil {
		return ctxResult, nil // missing profile → empty context, no error
	}

	ctxResult.Extra = make(map[string]interface{})

	for fieldName, fieldValue := range profile.Data {
		if fieldValue == nil {
			continue
		}
		val := fieldValue.Value
		if val == "" {
			continue
		}

		// Map known fields case-insensitively
		category, known := medicalFieldMapping[strings.ToLower(fieldName)]
		if !known {
			ctxResult.Extra[fieldName] = val
			continue
		}

		switch category {
		case "allergies":
			ctxResult.Allergies = append(ctxResult.Allergies, val)
		case "conditions":
			ctxResult.Conditions = append(ctxResult.Conditions, val)
		case "medications":
			ctxResult.Medications = append(ctxResult.Medications, val)
		case "vaccinations":
			ctxResult.Vaccinations = append(ctxResult.Vaccinations, val)
		case "blood_type":
			ctxResult.BloodType = val
		}
	}

	// If Extra is empty, set to nil for clean JSON
	if len(ctxResult.Extra) == 0 {
		ctxResult.Extra = nil
	}

	return ctxResult, nil
}

// =============================================================================
// GetTravelPreferences — preferencias de viaje → TravelAIContext
// =============================================================================

// GetTravelPreferences maps TravelPreferences 1:1 to TravelAIContext.
func (a *UserHealthAdapter) GetTravelPreferences(ctx context.Context, userID string) (*searchdomain.TravelAIContext, error) {
	ctxResult := &searchdomain.TravelAIContext{}

	if a.travelRepo == nil {
		return ctxResult, nil
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return ctxResult, nil
	}

	prefs, err := a.travelRepo.GetByUserID(ctx, uid)
	if err != nil || prefs == nil {
		return ctxResult, nil // missing prefs → empty context
	}

	ctxResult.PreferredClass = string(prefs.PreferredClass)
	if prefs.SeatPreference != nil {
		ctxResult.SeatPreference = string(*prefs.SeatPreference)
	}
	if prefs.MealPreference != nil {
		ctxResult.MealPreference = *prefs.MealPreference
	}
	ctxResult.SpecialAssistance = prefs.SpecialAssistance
	ctxResult.AvoidLayovers = prefs.AvoidLayovers
	if prefs.MaxLayoverDuration != nil {
		ctxResult.MaxLayoverDuration = *prefs.MaxLayoverDuration
	}

	return ctxResult, nil
}

// =============================================================================
// GetNationality — nacionalidad del perfil de usuario
// =============================================================================

// GetNationality reads the user's nationality from their profile.
// Returns empty string if the profile is missing or nationality is not set.
func (a *UserHealthAdapter) GetNationality(ctx context.Context, userID string) string {
	if a.profileRepo == nil {
		return ""
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return ""
	}
	profile, err := a.profileRepo.GetByUserID(ctx, uid)
	if err != nil || profile == nil {
		return ""
	}
	if profile.Nationality != nil {
		return *profile.Nationality
	}
	return ""
}

// =============================================================================
// GetDocumentContext — documentos de viaje → []DocumentContext
// =============================================================================

// travelDocumentCodes are the DocumentType codes that qualify as travel documents.
var travelDocumentCodes = map[string]bool{
	"passport": true,
	"visa":     true,
}

// knownDocumentFields maps known ExtractedData JSON keys to DocumentContext fields.
var knownDocumentFields = map[string]string{
	"document_number":  "number",
	"passport_number":  "number",
	"issuing_country":  "issuing_country",
	"country_code":     "issuing_country",
	"nationality":      "nationality",
	"valid_until":      "valid_until",
	"expiry_date":      "valid_until",
	"expiration_date":  "valid_until",
}

// GetDocumentContext reads the user's documents, filters by passport/visa types,
// and extracts common fields from ExtractedData JSONB.
func (a *UserHealthAdapter) GetDocumentContext(ctx context.Context, userID string) ([]searchdomain.DocumentContext, error) {
	if a.documentRepo == nil {
		return nil, nil
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, nil
	}

	// Resolve document types to build a code→typeID map
	types, err := a.documentRepo.GetTypes(ctx)
	if err != nil {
		return nil, nil
	}
	typeIDToCode := make(map[uuid.UUID]string, len(types))
	for _, dt := range types {
		typeIDToCode[dt.ID] = dt.Code
	}

	docs, err := a.documentRepo.GetByUserID(ctx, uid)
	if err != nil {
		return nil, err
	}

	var results []searchdomain.DocumentContext
	for _, doc := range docs {
		if doc.DocumentTypeID == nil {
			continue // skip documents sin tipo aún
		}
		code := typeIDToCode[*doc.DocumentTypeID]
		if !travelDocumentCodes[code] {
			continue // skip non-travel documents
		}

		ctxDoc := searchdomain.DocumentContext{
			Type: code,
		}

		// Extract known fields from ExtractedData JSONB
		if len(doc.ExtractedData) > 0 {
			var extracted map[string]interface{}
			if err := json.Unmarshal(doc.ExtractedData, &extracted); err == nil {
				extra := make(map[string]interface{})

				for key, value := range extracted {
					lowerKey := strings.ToLower(key)
					if field, known := knownDocumentFields[lowerKey]; known {
						if strVal, ok := value.(string); ok && strVal != "" {
							switch field {
							case "number":
								if ctxDoc.Number == "" {
									ctxDoc.Number = strVal
								}
							case "issuing_country":
								if ctxDoc.IssuingCountry == "" {
									ctxDoc.IssuingCountry = strVal
								}
							case "nationality":
								if ctxDoc.Nationality == "" {
									ctxDoc.Nationality = strVal
								}
							case "valid_until":
								if ctxDoc.ValidUntil == "" {
									ctxDoc.ValidUntil = strVal
								}
							}
						}
					} else {
						extra[key] = value
					}
				}

				if len(extra) > 0 {
					ctxDoc.Extra = extra
				}
			}
		}

		results = append(results, ctxDoc)
	}

	return results, nil
}
