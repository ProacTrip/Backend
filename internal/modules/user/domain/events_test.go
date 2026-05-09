package domain

import (
	"testing"

	"github.com/google/uuid"
)

// =============================================================================
// T-1.3: Domain Events — constructores y tipos
// =============================================================================

func TestUserProfileCreated_EventType(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	evt := NewUserProfileCreated(userID, "Europe/Madrid", "es", "EUR")

	if evt.EventType() != "UserProfileCreated" {
		t.Errorf("EventType = %s, se esperaba UserProfileCreated", evt.EventType())
	}
	if evt.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", evt.UserID, userID)
	}
	if evt.TimezoneName != "Europe/Madrid" {
		t.Errorf("TimezoneName = %s, se esperaba Europe/Madrid", evt.TimezoneName)
	}
	if evt.LanguageCode != "es" {
		t.Errorf("LanguageCode = %s, se esperaba es", evt.LanguageCode)
	}
	if evt.CurrencyCode != "EUR" {
		t.Errorf("CurrencyCode = %s, se esperaba EUR", evt.CurrencyCode)
	}
}

func TestUserProfileUpdated_EventType(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	evt := NewUserProfileUpdated(userID)

	if evt.EventType() != "UserProfileUpdated" {
		t.Errorf("EventType = %s, se esperaba UserProfileUpdated", evt.EventType())
	}
	if evt.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", evt.UserID, userID)
	}
}

func TestTravelPreferencesUpdated_EventType(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	evt := NewTravelPreferencesUpdated(userID)

	if evt.EventType() != "TravelPreferencesUpdated" {
		t.Errorf("EventType = %s, se esperaba TravelPreferencesUpdated", evt.EventType())
	}
}

func TestMedicalProfileUpdated_EventType(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	evt := NewMedicalProfileUpdated(userID)

	if evt.EventType() != "MedicalProfileUpdated" {
		t.Errorf("EventType = %s, se esperaba MedicalProfileUpdated", evt.EventType())
	}
}

func TestNotificationPreferencesUpdated_EventType(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	evt := NewNotificationPreferencesUpdated(userID)

	if evt.EventType() != "NotificationPreferencesUpdated" {
		t.Errorf("EventType = %s, se esperaba NotificationPreferencesUpdated", evt.EventType())
	}
}

func TestDocumentUploadReceived_EventType(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())
	evt := NewDocumentUploadReceived(userID, docID, "passport.pdf")

	if evt.EventType() != "DocumentUploadReceived" {
		t.Errorf("EventType = %s, se esperaba DocumentUploadReceived", evt.EventType())
	}
	if evt.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", evt.UserID, userID)
	}
	if evt.DocumentID != docID {
		t.Errorf("DocumentID = %v, se esperaba %v", evt.DocumentID, docID)
	}
}

func TestDocumentValidationPassed_EventType(t *testing.T) {
	evt := NewDocumentValidationPassed(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if evt.EventType() != "DocumentValidationPassed" {
		t.Errorf("EventType = %s, se esperaba DocumentValidationPassed", evt.EventType())
	}
}

func TestDocumentValidationFailed_EventType(t *testing.T) {
	evt := NewDocumentValidationFailed(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "invalid format")
	if evt.EventType() != "DocumentValidationFailed" {
		t.Errorf("EventType = %s, se esperaba DocumentValidationFailed", evt.EventType())
	}
	if evt.Reason != "invalid format" {
		t.Errorf("Reason = %s, se esperaba 'invalid format'", evt.Reason)
	}
}

func TestDocumentSanitized_EventType(t *testing.T) {
	evt := NewDocumentSanitized(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if evt.EventType() != "DocumentSanitized" {
		t.Errorf("EventType = %s, se esperaba DocumentSanitized", evt.EventType())
	}
}

func TestDocumentOCRCompleted_EventType(t *testing.T) {
	evt := NewDocumentOCRCompleted(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if evt.EventType() != "DocumentOCRCompleted" {
		t.Errorf("EventType = %s, se esperaba DocumentOCRCompleted", evt.EventType())
	}
}

func TestDocumentRejected_EventType(t *testing.T) {
	evt := NewDocumentRejected(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "invalid mime")
	if evt.EventType() != "DocumentRejected" {
		t.Errorf("EventType = %s, se esperaba DocumentRejected", evt.EventType())
	}
}

func TestDocumentDeleted_EventType(t *testing.T) {
	evt := NewDocumentDeleted(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if evt.EventType() != "DocumentDeleted" {
		t.Errorf("EventType = %s, se esperaba DocumentDeleted", evt.EventType())
	}
}

func TestDocumentVerified_EventType(t *testing.T) {
	evt := NewDocumentVerified(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if evt.EventType() != "DocumentVerified" {
		t.Errorf("EventType = %s, se esperaba DocumentVerified", evt.EventType())
	}
}

// Verificar que todos los eventos implementan DomainEvent interface
func TestDomainEvents_Interface(t *testing.T) {
	// Compile-time check: todos deben tener EventType() string
	events := []DomainEvent{
		NewUserProfileCreated(uuid.Must(uuid.NewV7()), "UTC", "es", "EUR"),
		NewUserProfileUpdated(uuid.Must(uuid.NewV7())),
		NewTravelPreferencesUpdated(uuid.Must(uuid.NewV7())),
		NewMedicalProfileUpdated(uuid.Must(uuid.NewV7())),
		NewNotificationPreferencesUpdated(uuid.Must(uuid.NewV7())),
		NewDocumentUploadReceived(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "file.pdf"),
		NewDocumentValidationPassed(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())),
		NewDocumentValidationFailed(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "reason"),
		NewDocumentSanitized(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())),
		NewDocumentOCRCompleted(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())),
		NewDocumentRejected(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "reason"),
		NewDocumentDeleted(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())),
		NewDocumentVerified(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())),
	}

	for i, evt := range events {
		if evt.EventType() == "" {
			t.Errorf("evento %d tiene EventType vacío", i)
		}
	}
}
