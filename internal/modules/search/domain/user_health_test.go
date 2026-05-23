package domain

import (
	"encoding/json"
	"testing"
)

// =============================================================================
// RED: Task 1.1 — UserHealthPort interface + DTOs
// These tests reference types that do NOT exist yet.
// =============================================================================

func TestMedicalAIContext_JSONTags(t *testing.T) {
	ctx := MedicalAIContext{
		Allergies:    []string{"Maní", "Polen"},
		Conditions:   []string{"Asma"},
		Medications:  []string{"Ibuprofeno"},
		Vaccinations: []string{"Fiebre amarilla"},
		BloodType:    "O+",
		Extra:        map[string]interface{}{"notas": "Paciente estable"},
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Verify known fields are serialized
	if _, ok := result["allergies"]; !ok {
		t.Error("allergies field not found in serialized JSON")
	}
	if _, ok := result["conditions"]; !ok {
		t.Error("conditions field not found in serialized JSON")
	}
	if _, ok := result["medications"]; !ok {
		t.Error("medications field not found in serialized JSON")
	}

	// Verify empty fields are omitted via omitzero
	if _, ok := result["blood_type"]; !ok {
		// This is OK if blood_type is empty — but we set it, so it should be present
		t.Error("blood_type should be present in serialized JSON")
	}
}

func TestMedicalAIContext_EmptyOmitzero(t *testing.T) {
	ctx := MedicalAIContext{}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Empty slices and zero values should be omitted
	if _, ok := result["allergies"]; ok {
		t.Error("empty allergies should be omitted with omitzero")
	}
	if _, ok := result["conditions"]; ok {
		t.Error("empty conditions should be omitted with omitzero")
	}
	if _, ok := result["blood_type"]; ok {
		t.Error("empty blood_type should be omitted with omitzero")
	}
}

func TestMedicalAlert_JSONTags(t *testing.T) {
	alert := MedicalAlert{
		Level:   "warning",
		Type:    "allergy",
		Message: "Alergia detectada: Maní",
	}

	data, err := json.Marshal(alert)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if result["level"] != "warning" {
		t.Errorf("level = %v, want 'warning'", result["level"])
	}
	if result["type"] != "allergy" {
		t.Errorf("type = %v, want 'allergy'", result["type"])
	}
	if result["message"] != "Alergia detectada: Maní" {
		t.Errorf("message = %v, want 'Alergia detectada: Maní'", result["message"])
	}
}

func TestTravelAIContext_JSONTags(t *testing.T) {
	ctx := TravelAIContext{
		PreferredClass:    "business",
		SeatPreference:    "window",
		MealPreference:    "vegetariano",
		SpecialAssistance: []string{"silla de ruedas"},
		AvoidLayovers:     true,
		MaxLayoverDuration: 90,
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if result["preferred_class"] != "business" {
		t.Errorf("preferred_class = %v, want 'business'", result["preferred_class"])
	}
	if result["avoid_layovers"] != true {
		t.Errorf("avoid_layovers = %v, want true", result["avoid_layovers"])
	}
}

func TestDocumentContext_JSONTags(t *testing.T) {
	ctx := DocumentContext{
		Type:           "passport",
		Number:         "A12345678",
		IssuingCountry: "PER",
		Nationality:    "PER",
		ValidUntil:     "2030-01-14",
		Extra:          map[string]interface{}{"name": "Juan Pérez"},
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if result["type"] != "passport" {
		t.Errorf("type = %v, want 'passport'", result["type"])
	}
	if result["number"] != "A12345678" {
		t.Errorf("number = %v, want 'A12345678'", result["number"])
	}
}

// Compile-time interface check: UserHealthPort is referenced.
func TestUserHealthPort_IsDefined(t *testing.T) {
	// This test only ensures the interface type exists and compiles.
	// The adapter's compile-time guard provides the actual satisfaction check.
	var port UserHealthPort = nil // nolint:staticcheck
	if false {
		_ = port
	}
	t.Log("UserHealthPort interface type exists")
}
