// Domain: Eventos de dominio para el módulo user.
// Define los eventos que el módulo emite para comunicación entre módulos.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// DomainEvent — Interfaz base para todos los eventos de dominio
// =============================================================================

// DomainEvent es la interfaz que todos los eventos de dominio deben implementar.
type DomainEvent interface {
	EventType() string
}

// =============================================================================
// UserProfileCreated — Emitido cuando se crea un perfil de usuario
// =============================================================================

type UserProfileCreated struct {
	UserID       uuid.UUID `json:"user_id"`
	TimezoneName string    `json:"timezone_name"`
	LanguageCode string    `json:"language_code"`
	CurrencyCode string    `json:"currency_code"`
	OccurredAt   time.Time `json:"occurred_at"`
}

func NewUserProfileCreated(userID uuid.UUID, timezone, language, currency string) *UserProfileCreated {
	return &UserProfileCreated{
		UserID:       userID,
		TimezoneName: timezone,
		LanguageCode: language,
		CurrencyCode: currency,
		OccurredAt:   time.Now(),
	}
}

func (e *UserProfileCreated) EventType() string { return "UserProfileCreated" }

// =============================================================================
// UserProfileUpdated — Emitido cuando se actualiza un perfil
// =============================================================================

type UserProfileUpdated struct {
	UserID     uuid.UUID `json:"user_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewUserProfileUpdated(userID uuid.UUID) *UserProfileUpdated {
	return &UserProfileUpdated{
		UserID:     userID,
		OccurredAt: time.Now(),
	}
}

func (e *UserProfileUpdated) EventType() string { return "UserProfileUpdated" }

// =============================================================================
// TravelPreferencesUpdated
// =============================================================================

type TravelPreferencesUpdated struct {
	UserID     uuid.UUID `json:"user_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewTravelPreferencesUpdated(userID uuid.UUID) *TravelPreferencesUpdated {
	return &TravelPreferencesUpdated{
		UserID:     userID,
		OccurredAt: time.Now(),
	}
}

func (e *TravelPreferencesUpdated) EventType() string { return "TravelPreferencesUpdated" }

// =============================================================================
// MedicalProfileUpdated
// =============================================================================

type MedicalProfileUpdated struct {
	UserID     uuid.UUID `json:"user_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewMedicalProfileUpdated(userID uuid.UUID) *MedicalProfileUpdated {
	return &MedicalProfileUpdated{
		UserID:     userID,
		OccurredAt: time.Now(),
	}
}

func (e *MedicalProfileUpdated) EventType() string { return "MedicalProfileUpdated" }

// =============================================================================
// NotificationPreferencesUpdated
// =============================================================================

type NotificationPreferencesUpdated struct {
	UserID     uuid.UUID `json:"user_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewNotificationPreferencesUpdated(userID uuid.UUID) *NotificationPreferencesUpdated {
	return &NotificationPreferencesUpdated{
		UserID:     userID,
		OccurredAt: time.Now(),
	}
}

func (e *NotificationPreferencesUpdated) EventType() string { return "NotificationPreferencesUpdated" }

// =============================================================================
// Document Events — Pipeline de procesamiento de documentos
// =============================================================================

// DocumentUploadReceived — Documento recibido, inicia pipeline
type DocumentUploadReceived struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	FileName   string    `json:"file_name"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewDocumentUploadReceived(userID, documentID uuid.UUID, fileName string) *DocumentUploadReceived {
	return &DocumentUploadReceived{
		UserID:     userID,
		DocumentID: documentID,
		FileName:   fileName,
		OccurredAt: time.Now(),
	}
}

func (e *DocumentUploadReceived) EventType() string { return "DocumentUploadReceived" }

// DocumentValidationPassed — Validación superada
type DocumentValidationPassed struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewDocumentValidationPassed(userID, documentID uuid.UUID) *DocumentValidationPassed {
	return &DocumentValidationPassed{
		UserID:     userID,
		DocumentID: documentID,
		OccurredAt: time.Now(),
	}
}

func (e *DocumentValidationPassed) EventType() string { return "DocumentValidationPassed" }

// DocumentValidationFailed — Validación fallida
type DocumentValidationFailed struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewDocumentValidationFailed(userID, documentID uuid.UUID, reason string) *DocumentValidationFailed {
	return &DocumentValidationFailed{
		UserID:     userID,
		DocumentID: documentID,
		Reason:     reason,
		OccurredAt: time.Now(),
	}
}

func (e *DocumentValidationFailed) EventType() string { return "DocumentValidationFailed" }

// DocumentSanitized — Documento saneado (limpieza de metadatos)
type DocumentSanitized struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewDocumentSanitized(userID, documentID uuid.UUID) *DocumentSanitized {
	return &DocumentSanitized{
		UserID:     userID,
		DocumentID: documentID,
		OccurredAt: time.Now(),
	}
}

func (e *DocumentSanitized) EventType() string { return "DocumentSanitized" }

// DocumentOCRCompleted — OCR completado exitosamente
type DocumentOCRCompleted struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewDocumentOCRCompleted(userID, documentID uuid.UUID) *DocumentOCRCompleted {
	return &DocumentOCRCompleted{
		UserID:     userID,
		DocumentID: documentID,
		OccurredAt: time.Now(),
	}
}

func (e *DocumentOCRCompleted) EventType() string { return "DocumentOCRCompleted" }

// DocumentRejected — Documento rechazado
type DocumentRejected struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewDocumentRejected(userID, documentID uuid.UUID, reason string) *DocumentRejected {
	return &DocumentRejected{
		UserID:     userID,
		DocumentID: documentID,
		Reason:     reason,
		OccurredAt: time.Now(),
	}
}

func (e *DocumentRejected) EventType() string { return "DocumentRejected" }

// DocumentDeleted — Documento eliminado
type DocumentDeleted struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewDocumentDeleted(userID, documentID uuid.UUID) *DocumentDeleted {
	return &DocumentDeleted{
		UserID:     userID,
		DocumentID: documentID,
		OccurredAt: time.Now(),
	}
}

func (e *DocumentDeleted) EventType() string { return "DocumentDeleted" }

// DocumentVerified — Documento verificado por admin
type DocumentVerified struct {
	UserID     uuid.UUID `json:"user_id"`
	DocumentID uuid.UUID `json:"document_id"`
	VerifiedBy uuid.UUID `json:"verified_by"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewDocumentVerified(userID, documentID, verifiedBy uuid.UUID) *DocumentVerified {
	return &DocumentVerified{
		UserID:     userID,
		DocumentID: documentID,
		VerifiedBy: verifiedBy,
		OccurredAt: time.Now(),
	}
}

func (e *DocumentVerified) EventType() string { return "DocumentVerified" }

// =============================================================================
// LocaleUpdated — Emitido cuando se actualiza timezone, idioma o moneda
// =============================================================================

type LocaleUpdated struct {
	UserID       uuid.UUID `json:"user_id"`
	TimezoneName string    `json:"timezone_name"`
	LanguageCode string    `json:"language_code"`
	CurrencyCode string    `json:"currency_code"`
	OccurredAt   time.Time `json:"occurred_at"`
}

func NewLocaleUpdated(userID uuid.UUID, timezone, language, currency string) *LocaleUpdated {
	return &LocaleUpdated{
		UserID:       userID,
		TimezoneName: timezone,
		LanguageCode: language,
		CurrencyCode: currency,
		OccurredAt:   time.Now(),
	}
}

func (e *LocaleUpdated) EventType() string { return "LocaleUpdated" }

// =============================================================================
// MedicalPendingResolved — Emitido cuando se resuelve un conflicto médico
// =============================================================================

type MedicalPendingResolved struct {
	UserID     uuid.UUID `json:"user_id"`
	PendingID  uuid.UUID `json:"pending_id"`
	Status     string    `json:"status"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewMedicalPendingResolved(userID, pendingID uuid.UUID, status string) *MedicalPendingResolved {
	return &MedicalPendingResolved{
		UserID:     userID,
		PendingID:  pendingID,
		Status:     status,
		OccurredAt: time.Now(),
	}
}

func (e *MedicalPendingResolved) EventType() string { return "MedicalPendingResolved" }
