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
// T-1.1: Medical Profile V2
// =============================================================================

func TestNewMedicalProfileV2(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mp := NewMedicalProfileV2(userID)

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

func TestMedicalProfileV2_SetField(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mp := NewMedicalProfileV2(userID)
	oldUpdated := mp.UpdatedAt

	time.Sleep(1 * time.Millisecond)
	mp.SetField("blood_type", "A+", MedicalSourceProfile)

	val, exists := mp.Data["blood_type"]
	if !exists {
		t.Fatal("campo blood_type debería existir")
	}
	if val.Value != "A+" {
		t.Errorf("Value = %s, se esperaba A+", val.Value)
	}
	if val.Source != MedicalSourceProfile {
		t.Errorf("Source = %s, se esperaba profile", val.Source)
	}
	if val.UpdatedAt.IsZero() {
		t.Error("UpdatedAt del campo no debería ser zero")
	}
	if !mp.UpdatedAt.After(oldUpdated) {
		t.Error("UpdatedAt del perfil debería haberse actualizado")
	}
}

func TestMedicalProfileV2_RemoveField(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mp := NewMedicalProfileV2(userID)
	mp.SetField("blood_type", "A+", MedicalSourceProfile)
	mp.RemoveField("blood_type")

	if _, exists := mp.Data["blood_type"]; exists {
		t.Error("campo blood_type debería haberse eliminado")
	}
}

func TestMedicalProfileV2_JSON(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mp := NewMedicalProfileV2(userID)
	mp.SetField("blood_type", "A+", MedicalSourceProfile)

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
// T-1.1: Notification Preference
// =============================================================================

func TestNewNotificationPreference(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	np := NewNotificationPreference(userID, NotifChannelEmail, NotifTypeBookingConfirmation)

	if np.ID == uuid.Nil {
		t.Error("se esperaba UUIDv7 no nulo")
	}
	if np.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", np.UserID, userID)
	}
	if np.Channel != NotifChannelEmail {
		t.Errorf("Channel = %s, se esperaba email", np.Channel)
	}
	if np.NotificationType != NotifTypeBookingConfirmation {
		t.Errorf("NotificationType = %s, se esperaba booking_confirmation", np.NotificationType)
	}
	if np.Enabled != true {
		t.Error("Enabled debería ser true por defecto")
	}
}

func TestNotificationPreference_Toggle(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	np := NewNotificationPreference(userID, NotifChannelEmail, NotifTypeBookingConfirmation)
	oldUpdated := np.UpdatedAt

	time.Sleep(1 * time.Millisecond)
	np.Toggle()

	if np.Enabled != false {
		t.Error("Enabled debería ser false después de toggle")
	}
	if !np.UpdatedAt.After(oldUpdated) {
		t.Error("UpdatedAt debería haberse actualizado")
	}

	np.Toggle()
	if np.Enabled != true {
		t.Error("Enabled debería ser true después del segundo toggle")
	}
}

// =============================================================================
// T-1.1: Notification Channel / Type enums
// =============================================================================

func TestNotificationChannel_Values(t *testing.T) {
	channels := []NotificationChannel{NotifChannelEmail, NotifChannelSMS, NotifChannelWebSocket}
	expected := []string{"email", "sms", "websocket"}
	for i, ch := range channels {
		if string(ch) != expected[i] {
			t.Errorf("NotificationChannel[%d] = %s, se esperaba %s", i, ch, expected[i])
		}
	}
}

func TestNotificationType_Values(t *testing.T) {
	types := []NotificationType{NotifTypeBookingConfirmation, NotifTypeFlightReminder, NotifTypePromotional}
	expected := []string{"booking_confirmation", "flight_reminder", "promotional"}
	for i, nt := range types {
		if string(nt) != expected[i] {
			t.Errorf("NotificationType[%d] = %s, se esperaba %s", i, nt, expected[i])
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
	if decoded["is_verified"] != false {
		t.Error("is_verified debería ser false")
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

// =============================================================================
// T-1.1: Saved Search
// =============================================================================

func TestNewSavedSearch(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	params := map[string]any{"origin": "MAD", "destination": "BCN"}
	search, err := NewSavedSearch(userID, "My Search", params, "test-hash-123")
	if err != nil {
		t.Fatal(err)
	}

	if search.ID == uuid.Nil {
		t.Error("se esperaba UUIDv7 no nulo")
	}
	if search.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", search.UserID, userID)
	}
	if search.AlertEnabled != false {
		t.Error("AlertEnabled debería ser false por defecto")
	}
	if search.SearchHash != "test-hash-123" {
		t.Errorf("SearchHash = %s, se esperaba test-hash-123", search.SearchHash)
	}
}

func TestSavedSearch_ToggleAlert(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	search, err := NewSavedSearch(userID, "Test", map[string]any{"q": "test"}, "toggle-hash")
	if err != nil {
		t.Fatal(err)
	}
	oldUpdated := search.UpdatedAt

	time.Sleep(1 * time.Millisecond)
	search.ToggleAlert()

	if search.AlertEnabled != true {
		t.Error("AlertEnabled debería ser true después de toggle")
	}
	if !search.UpdatedAt.After(oldUpdated) {
		t.Error("UpdatedAt debería haberse actualizado")
	}
}

// =============================================================================
// T-1.1: Favorite
// =============================================================================

func TestNewFavorite(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	entityID := uuid.Must(uuid.NewV7())
	fav := NewFavorite(userID, entityID, FavoriteEntityHotel, "Hotel Ritz")

	if fav.ID == uuid.Nil {
		t.Error("se esperaba UUIDv7 no nulo")
	}
	if fav.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", fav.UserID, userID)
	}
	if fav.EntityID != entityID {
		t.Errorf("EntityID = %v, se esperaba %v", fav.EntityID, entityID)
	}
	if fav.EntityType != FavoriteEntityHotel {
		t.Errorf("EntityType = %s, se esperaba hotel", fav.EntityType)
	}
	if fav.Title != "Hotel Ritz" {
		t.Errorf("Title = %s, se esperaba Hotel Ritz", fav.Title)
	}
}

func TestFavorite_SetNotes(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	fav := NewFavorite(userID, uuid.Must(uuid.NewV7()), FavoriteEntityFlight, "Vuelo a NYC")
	oldUpdated := fav.UpdatedAt

	time.Sleep(1 * time.Millisecond)
	notes := "Vuelo directo, salida por la mañana"
	fav.SetNotes(&notes)

	if fav.Notes == nil || *fav.Notes != notes {
		t.Errorf("Notes = %v, se esperaba %s", fav.Notes, notes)
	}
	if !fav.UpdatedAt.After(oldUpdated) {
		t.Error("UpdatedAt debería haberse actualizado")
	}
}

func TestFavorite_JSON(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	fav := NewFavorite(userID, uuid.Must(uuid.NewV7()), FavoriteEntityActivity, "Safari en Kenia")

	data, err := json.Marshal(fav)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded["entity_type"] != "activity" {
		t.Errorf("entity_type = %v, se esperaba activity", decoded["entity_type"])
	}
	if decoded["title"] != "Safari en Kenia" {
		t.Errorf("title = %v, se esperaba Safari en Kenia", decoded["title"])
	}
	// notes debe omitirse cuando es nil
	if _, exists := decoded["notes"]; exists {
		t.Error("notes nil debería omitirse con omitzero")
	}
}

// =============================================================================
// T-1.1: Favorite Entity Type enum
// =============================================================================

func TestFavoriteEntityType_Values(t *testing.T) {
	types := []FavoriteEntityType{
		FavoriteEntityHotel, FavoriteEntityFlight, FavoriteEntityActivity,
	}
	expected := []string{
		"hotel", "flight", "activity",
	}
	for i, et := range types {
		if string(et) != expected[i] {
			t.Errorf("FavoriteEntityType[%d] = %s, se esperaba %s", i, et, expected[i])
		}
	}
}

func TestIsValidFavoriteEntityType(t *testing.T) {
	valid := []string{"hotel", "flight", "activity"}
	for _, v := range valid {
		if !IsValidFavoriteEntityType(v) {
			t.Errorf("IsValidFavoriteEntityType(%q) debería ser true", v)
		}
	}

	invalid := []string{"", "car", "train", "invalid", "airport", "airline", "hotel_chain", "country", "destination"}
	for _, iv := range invalid {
		if IsValidFavoriteEntityType(iv) {
			t.Errorf("IsValidFavoriteEntityType(%q) debería ser false", iv)
		}
	}
}


