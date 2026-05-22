// Tests de dominio para entidades del módulo user.
// Verifica factories, JSON serialización y métodos de comportamiento.
package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// T-1.1: Travel Preferences
// =============================================================================

func TestNewTravelPreferences(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	prefs := NewTravelPreferences(userID)

	if prefs.ID == uuid.Nil {
		t.Error("se esperaba UUIDv7 no nulo")
	}
	if prefs.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", prefs.UserID, userID)
	}
	if prefs.PreferredClass != CabinClassEconomy {
		t.Errorf("PreferredClass = %s, se esperaba %s", prefs.PreferredClass, CabinClassEconomy)
	}
	if prefs.AvoidLayovers != false {
		t.Error("AvoidLayovers debería ser false por defecto")
	}
	if prefs.CreatedAt.IsZero() || prefs.UpdatedAt.IsZero() {
		t.Error("timestamps no deberían ser zero")
	}
}

func TestTravelPreferences_SetClasses(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	prefs := NewTravelPreferences(userID)
	oldUpdated := prefs.UpdatedAt

	time.Sleep(1 * time.Millisecond)
	prefs.SetClasses(CabinClassBusiness, SeatAisle)

	if prefs.PreferredClass != CabinClassBusiness {
		t.Errorf("PreferredClass = %s, se esperaba business", prefs.PreferredClass)
	}
	if *prefs.SeatPreference != SeatAisle {
		t.Errorf("SeatPreference = %v, se esperaba aisle", *prefs.SeatPreference)
	}
	if !prefs.UpdatedAt.After(oldUpdated) {
		t.Error("UpdatedAt debería haberse actualizado")
	}
}

func TestTravelPreferences_JSON(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	prefs := NewTravelPreferences(userID)
	prefs.PreferredClass = CabinClassFirst
	seatWin := SeatWindow
	prefs.SeatPreference = &seatWin
	prefs.AvoidLayovers = true

	data, err := json.Marshal(prefs)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded["preferred_class"] != "first" {
		t.Errorf("preferred_class = %v, se esperaba first", decoded["preferred_class"])
	}
	if decoded["avoid_layovers"] != true {
		t.Error("avoid_layovers debería ser true")
	}
	// omitzero: nil fields deben omitirse
	if _, exists := decoded["meal_preference"]; exists {
		t.Error("meal_preference nil debería omitirse con omitzero")
	}
}

// =============================================================================
// T-1.1: CabinClass / SeatPreference enums
// =============================================================================

func TestCabinClass_Values(t *testing.T) {
	classes := []CabinClass{CabinClassEconomy, CabinClassPremiumEconomy, CabinClassBusiness, CabinClassFirst}
	expected := []string{"economy", "premium_economy", "business", "first"}
	for i, c := range classes {
		if string(c) != expected[i] {
			t.Errorf("CabinClass[%d] = %s, se esperaba %s", i, c, expected[i])
		}
	}
}

func TestSeatPreference_Values(t *testing.T) {
	seats := []SeatPreference{SeatWindow, SeatAisle, SeatMiddle, SeatNoPreference}
	expected := []string{"window", "aisle", "middle", "no_preference"}
	for i, s := range seats {
		if string(s) != expected[i] {
			t.Errorf("SeatPreference[%d] = %s, se esperaba %s", i, s, expected[i])
		}
	}
}

// =============================================================================
// T-1.1: Medical Profile
// =============================================================================

func TestNewMedicalProfile(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mp := NewMedicalProfile(userID)

	if mp.ID == uuid.Nil {
		t.Error("se esperaba UUIDv7 no nulo")
	}
	if mp.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", mp.UserID, userID)
	}
	if mp.Data == nil {
		t.Error("Data no debería ser nil")
	}
	if len(mp.Data) != 0 {
		t.Errorf("Data debería estar vacío, tiene %d elementos", len(mp.Data))
	}
	if mp.IsShared != false {
		t.Error("IsShared debería ser false por defecto")
	}
}

func TestMedicalProfile_SetField(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mp := NewMedicalProfile(userID)
	oldUpdated := mp.UpdatedAt

	time.Sleep(1 * time.Millisecond)
	source := MedicalSourceDetail{Type: "manual"}
	mp.SetField("blood_type", "A+", source)

	val, exists := mp.Data["blood_type"]
	if !exists {
		t.Fatal("campo blood_type debería existir")
	}
	if val.Value != "A+" {
		t.Errorf("Value = %s, se esperaba A+", val.Value)
	}
	if val.Source.Type != "manual" {
		t.Errorf("Source.Type = %s, se esperaba manual", val.Source.Type)
	}
	if val.UpdatedAt.IsZero() {
		t.Error("UpdatedAt del campo no debería ser zero")
	}
	if !mp.UpdatedAt.After(oldUpdated) {
		t.Error("UpdatedAt del perfil debería haberse actualizado")
	}
}

func TestMedicalProfile_RemoveField(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mp := NewMedicalProfile(userID)
	mp.SetField("blood_type", "A+", MedicalSourceDetail{Type: "manual"})
	mp.RemoveField("blood_type")

	if _, exists := mp.Data["blood_type"]; exists {
		t.Error("campo blood_type debería haberse eliminado")
	}
}

func TestMedicalProfile_JSON(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mp := NewMedicalProfile(userID)
	mp.SetField("blood_type", "A+", MedicalSourceDetail{Type: "manual"})

	data, err := json.Marshal(mp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	dataMap, ok := decoded["data"].(map[string]any)
	if !ok {
		t.Fatal("data debería ser un objeto")
	}
	bloodType, ok := dataMap["blood_type"].(map[string]any)
	if !ok {
		t.Fatal("data.blood_type debería ser un objeto")
	}
	if bloodType["value"] != "A+" {
		t.Errorf("data.blood_type.value = %v, se esperaba A+", bloodType["value"])
	}
}

// =============================================================================
// T-1.1: Medical Sources enum
// =============================================================================

func TestMedicalSource_Values(t *testing.T) {
	sources := []MedicalSource{MedicalSourceProfile, MedicalSourceOCR, MedicalSourceNLP}
	expected := []string{"profile", "ocr", "nlp"}
	for i, s := range sources {
		if string(s) != expected[i] {
			t.Errorf("MedicalSource[%d] = %s, se esperaba %s", i, s, expected[i])
		}
	}
}

// =============================================================================
// T-1.1: User Document
// =============================================================================

func TestNewUserDocument(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := NewUserDocument(userID, docTypeID, "passport.pdf", "keys/docs/abc123", "application/pdf")

	if doc.ID == uuid.Nil {
		t.Error("se esperaba UUIDv7 no nulo")
	}
	if doc.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", doc.UserID, userID)
	}
	if doc.DocumentTypeID != docTypeID {
		t.Errorf("DocumentTypeID = %v, se esperaba %v", doc.DocumentTypeID, docTypeID)
	}
	if doc.OCRStatus != OCRStatusQueued {
		t.Errorf("OCRStatus = %s, se esperaba queued", doc.OCRStatus)
	}
	if doc.FileName != "passport.pdf" {
		t.Errorf("FileName = %s, se esperaba passport.pdf", doc.FileName)
	}
}

func TestUserDocument_MarkValidationPassed(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	doc := NewUserDocument(userID, uuid.Must(uuid.NewV7()), "doc.pdf", "key", "application/pdf")
	oldUpdated := doc.UpdatedAt

	time.Sleep(1 * time.Millisecond)
	doc.MarkValidationPassed("passport")

	if doc.OCRStatus != OCRStatusValidating {
		t.Errorf("OCRStatus = %s, se esperaba validating", doc.OCRStatus)
	}
	if doc.DocumentType == nil || *doc.DocumentType != "passport" {
		t.Errorf("DocumentType = %v, se esperaba passport", doc.DocumentType)
	}
	if !doc.UpdatedAt.After(oldUpdated) {
		t.Error("UpdatedAt debería haberse actualizado")
	}
}

func TestUserDocument_JSON(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	doc := NewUserDocument(userID, uuid.Must(uuid.NewV7()), "doc.pdf", "key", "application/pdf")

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded["ocr_status"] != "queued" {
		t.Errorf("ocr_status = %v, se esperaba queued", decoded["ocr_status"])
	}

}

// =============================================================================
// T-1.1: OCR Status enum
// =============================================================================

func TestOCRStatus_Values(t *testing.T) {
	statuses := []OCRStatus{
		OCRStatusQueued, OCRStatusProcessing, OCRStatusValidating, OCRStatusSanitizing,
		OCRStatusOCRProcessing, OCRStatusCompleted, OCRStatusRejected, OCRStatusFailed,
	}
	expected := []string{"queued", "processing", "validating", "sanitizing", "ocr_processing", "completed", "rejected", "failed"}
	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("OCRStatus[%d] = %s, se esperaba %s", i, s, expected[i])
		}
	}
}

// =============================================================================
// T-1.1: Document Type
// =============================================================================

func TestDocumentType_JSON(t *testing.T) {
	dt := DocumentType{
		Code:        "passport",
		Name:        "Passport",
		IsIdentity:  true,
		RequiresOCR: true,
		SortOrder:   1,
	}
	data, err := json.Marshal(dt)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded["code"] != "passport" {
		t.Errorf("code = %v, se esperaba passport", decoded["code"])
	}
	if decoded["is_identity"] != true {
		t.Error("is_identity debería ser true")
	}
}
