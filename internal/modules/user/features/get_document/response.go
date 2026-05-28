// DTO de respuesta para GET /v1/user/profile/documents/:document_id — detalle de documento.
// Alineado con USER_API.md: incluye verified_at, verified_by y document_type como string.
package get_document

import (
	"encoding/json"
)

// DocumentDetailResponse representa un documento individual en respuesta de detalle.
// A diferencia del listado, incluye verified_at + verified_by y omite document_type_id UUID.
// Alineado con Frontend DocumentDetail interface (document.ts).
type DocumentDetailResponse struct {
	ID                 string           `json:"id"`
	UserID             string           `json:"user_id"`
	FileName           string           `json:"file_name"`
	FileSize           *int             `json:"file_size,omitzero"`
	MimeType           *string          `json:"mime_type,omitzero"`
	DetectedMimeType   *string          `json:"detected_mime_type,omitzero"`
	DetectedSizeBytes  *int64           `json:"detected_size_bytes,omitzero"`
	DocumentType       *string          `json:"document_type,omitzero"`
	StorageKey         string           `json:"storage_key"`
	OCRStatus          string           `json:"ocr_status"`
	OCRConfidence      *float64         `json:"ocr_confidence,omitzero"`
	ExtractedData      json.RawMessage  `json:"extracted_data,omitzero"`
	FailureReason      *string          `json:"failure_reason,omitzero"`
	VerificationStatus string           `json:"verification_status"`
	CreatedAt          string           `json:"created_at"`
	UpdatedAt          string           `json:"updated_at"`
	VerifiedAt         *string          `json:"verified_at,omitzero"`
	VerifiedBy         *string          `json:"verified_by,omitzero"`
}
