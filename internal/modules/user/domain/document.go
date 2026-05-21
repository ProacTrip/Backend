// Domain: Documentos de usuario.
// Define tipos de documento, estado OCR y metadatos.
package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Enums
// =============================================================================

// OCRStatus representa el estado del pipeline OCR de un documento.
type OCRStatus string

const (
	OCRStatusQueued        OCRStatus = "queued"
	OCRStatusProcessing    OCRStatus = "processing"
	OCRStatusValidating    OCRStatus = "validating"
	OCRStatusSanitizing    OCRStatus = "sanitizing"
	OCRStatusOCRProcessing OCRStatus = "ocr_processing"
	OCRStatusCompleted     OCRStatus = "completed"
	OCRStatusRejected      OCRStatus = "rejected"
	OCRStatusFailed        OCRStatus = "failed"
)

// DocumentVerificationStatus representa el estado de verificación de un documento.
type DocumentVerificationStatus string

const (
	VerificationStatusUnverified   DocumentVerificationStatus = "unverified"
	VerificationStatusVerified     DocumentVerificationStatus = "verified"
	VerificationStatusRejected     DocumentVerificationStatus = "rejected"
	VerificationStatusManualReview DocumentVerificationStatus = "manual_review"
	VerificationStatusSuspicious   DocumentVerificationStatus = "suspicious"
)

// =============================================================================
// DocumentType — Catálogo de tipos de documento (read-only)
// =============================================================================

// DocumentType representa un tipo de documento del catálogo.
// Alineado con la migración document_types.
type DocumentType struct {
	ID          uuid.UUID       `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitzero"`
	IsIdentity  bool            `json:"is_identity"`
	RequiresOCR bool            `json:"requires_ocr"`
	OCRFields   json.RawMessage `json:"ocr_fields,omitzero"`
	IsActive    bool            `json:"is_active"`
	SortOrder   int             `json:"sort_order"`
	CreatedAt   time.Time       `json:"created_at"`
}

// =============================================================================
// UserDocument — Documento de un usuario
// =============================================================================

// UserDocument representa un documento subido por el usuario.
// Alineado con la migración user_documents.
// Note: verified_at/verified_by removed — verification now lives in DASHBOARD module.
type UserDocument struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	DocumentTypeID uuid.UUID `json:"document_type_id"`
	FileName       string    `json:"file_name"`
	FileSize       *int      `json:"file_size,omitzero"`
	MimeType       *string   `json:"mime_type,omitzero"`
	StorageKey     string    `json:"storage_key"`

	// Pipeline V3: validación asíncrona
	DetectedMimeType  *string `json:"detected_mime_type,omitzero"`
	DetectedSizeBytes *int64  `json:"detected_size_bytes,omitzero"`
	DocumentType      *string `json:"document_type,omitzero"` // categoría detectada
	FailureReason     *string `json:"failure_reason,omitzero"`

	VerificationStatus DocumentVerificationStatus `json:"verification_status,omitzero"`

	OCRStatus     OCRStatus       `json:"ocr_status"`
	OCRData       json.RawMessage `json:"ocr_data,omitzero"`
	OCRConfidence *float64        `json:"ocr_confidence,omitzero"`
	ExtractedData json.RawMessage `json:"extracted_data,omitzero"`

	// Integración con perfil médico
	HasNewerMedicalData bool            `json:"has_newer_medical_data"`
	MedicalUpdateSummary json.RawMessage `json:"medical_update_summary,omitzero"`

	ValidFrom      *time.Time     `json:"valid_from,omitzero"`
	ValidUntil     *time.Time     `json:"valid_until,omitzero"`
	DocumentNumber *string        `json:"document_number,omitzero"`
	IssuingCountry *string        `json:"issuing_country,omitzero"`
	Metadata       json.RawMessage `json:"metadata,omitzero"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// NewUserDocument crea un nuevo documento en estado queued.
func NewUserDocument(userID, docTypeID uuid.UUID, fileName, storageKey, mimeType string) *UserDocument {
	now := time.Now()
	return &UserDocument{
		ID:                 uuid.Must(uuid.NewV7()),
		UserID:             userID,
		DocumentTypeID:     docTypeID,
		FileName:           fileName,
		StorageKey:         storageKey,
		MimeType:           &mimeType,
		OCRStatus:          OCRStatusQueued,
		VerificationStatus: VerificationStatusUnverified,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// MarkValidationPassed marca el documento como validado y avanza a estado validating.
func (d *UserDocument) MarkValidationPassed(detectedDocType string) {
	d.OCRStatus = OCRStatusValidating
	d.DocumentType = &detectedDocType
	d.UpdatedAt = time.Now()
}

// MarkValidationFailed marca el documento con fallo de validación.
func (d *UserDocument) MarkValidationFailed(reason string) {
	d.OCRStatus = OCRStatusRejected
	d.FailureReason = &reason
	d.UpdatedAt = time.Now()
}

// MarkOCRCompleted marca el OCR como completado.
func (d *UserDocument) MarkOCRCompleted(ocrData json.RawMessage, confidence float64) {
	d.OCRStatus = OCRStatusCompleted
	d.OCRData = ocrData
	d.OCRConfidence = &confidence
	d.UpdatedAt = time.Now()
}
