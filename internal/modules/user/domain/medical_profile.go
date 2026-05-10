// Domain: Perfil médico del usuario.
// Usa JSONB para datos flexibles, actualizaciones pendientes para conflictos OCR/NLP.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Enums
// =============================================================================

// MedicalSource indica el origen de un dato médico.
type MedicalSource string

const (
	MedicalSourceProfile MedicalSource = "profile" // ingresado manualmente
	MedicalSourceOCR     MedicalSource = "ocr"     // extraído de documento
	MedicalSourceNLP     MedicalSource = "nlp"     // extraído de conversación
)

// =============================================================================
// MedicalFieldValue — Valor de un campo médico con trazabilidad
// =============================================================================

// MedicalFieldValue representa un valor individual del perfil médico.
type MedicalFieldValue struct {
	Value     string        `json:"value"`
	Source    MedicalSource `json:"source"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// =============================================================================
// MedicalProfileV2 — Perfil médico v2 (JSONB flexible)
// =============================================================================

// MedicalProfileV2 representa el perfil médico del usuario.
// Alineado con la migración user_medical_profiles.
// Los datos se almacenan como JSONB (map[string]*MedicalFieldValue).
type MedicalProfileV2 struct {
	ID        uuid.UUID                         `json:"id"`
	UserID    uuid.UUID                         `json:"user_id"`
	Data      map[string]*MedicalFieldValue     `json:"data"`
	IsShared  bool                              `json:"is_shared"`
	CreatedAt time.Time                         `json:"created_at"`
	UpdatedAt time.Time                         `json:"updated_at"`
}

// NewMedicalProfileV2 crea un nuevo perfil médico vacío.
func NewMedicalProfileV2(userID uuid.UUID) *MedicalProfileV2 {
	now := time.Now()
	return &MedicalProfileV2{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		Data:      make(map[string]*MedicalFieldValue),
		IsShared:  false,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// SetField establece o actualiza un campo médico.
func (mp *MedicalProfileV2) SetField(fieldName, value string, source MedicalSource) {
	mp.Data[fieldName] = &MedicalFieldValue{
		Value:     value,
		Source:    source,
		UpdatedAt: time.Now(),
	}
	mp.UpdatedAt = time.Now()
}

// RemoveField elimina un campo del perfil médico.
func (mp *MedicalProfileV2) RemoveField(fieldName string) {
	delete(mp.Data, fieldName)
	mp.UpdatedAt = time.Now()
}

// =============================================================================
// MedicalPendingUpdate — Actualización médica pendiente de revisión
// =============================================================================

// MedicalPendingUpdateStatus representa el estado de una actualización pendiente.
type MedicalPendingUpdateStatus string

const (
	PendingUpdatePending  MedicalPendingUpdateStatus = "pending"
	PendingUpdateAccepted MedicalPendingUpdateStatus = "accepted"
	PendingUpdateRejected MedicalPendingUpdateStatus = "rejected"
)

// MedicalPendingUpdate representa un conflicto detectado entre
// datos OCR/NLP y el perfil médico actual.
type MedicalPendingUpdate struct {
	ID               uuid.UUID                    `json:"id"`
	UserID           uuid.UUID                    `json:"user_id"`
	SourceType       string                       `json:"source_type"`        // "ocr" o "nlp"
	SourceDocumentID *uuid.UUID                   `json:"source_document_id,omitzero"`
	SourceFileName   *string                      `json:"source_file_name,omitzero"`
	ConversationID   *uuid.UUID                   `json:"conversation_id,omitzero"`
	FieldName        string                       `json:"field_name"`
	CurrentValue     *string                      `json:"current_value,omitzero"`
	ProposedValue    string                       `json:"proposed_value"`
	SuggestedAt      time.Time                    `json:"suggested_at"`
	ExpiresAt        time.Time                    `json:"expires_at"`
	Status           MedicalPendingUpdateStatus   `json:"status"`
	ResolvedAt       *time.Time                   `json:"resolved_at,omitzero"`
}

// NewMedicalPendingUpdate crea una actualización pendiente.
func NewMedicalPendingUpdate(userID uuid.UUID, sourceType, fieldName, proposedValue string) *MedicalPendingUpdate {
	now := time.Now()
	return &MedicalPendingUpdate{
		ID:            uuid.Must(uuid.NewV7()),
		UserID:        userID,
		SourceType:    sourceType,
		FieldName:     fieldName,
		ProposedValue: proposedValue,
		SuggestedAt:   now,
		ExpiresAt:     now.Add(30 * 24 * time.Hour), // 30 días
		Status:        PendingUpdatePending,
	}
}

// Accept marca la actualización como aceptada.
func (pu *MedicalPendingUpdate) Accept() {
	pu.Status = PendingUpdateAccepted
	now := time.Now()
	pu.ResolvedAt = &now
}

// Reject marca la actualización como rechazada.
func (pu *MedicalPendingUpdate) Reject() {
	pu.Status = PendingUpdateRejected
	now := time.Now()
	pu.ResolvedAt = &now
}
