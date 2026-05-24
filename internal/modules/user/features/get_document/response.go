// DTO de respuesta para GET /v1/user/profile/documents/:document_id — detalle de documento.
// Alineado con USER_API.md: incluye verified_at, verified_by y document_type como string.
package get_document

// DocumentDetailResponse representa un documento individual en respuesta de detalle.
// A diferencia del listado, incluye verified_at + verified_by y omite document_type_id UUID.
type DocumentDetailResponse struct {
	ID                 string   `json:"id"`
	FileName           string   `json:"file_name"`
	DocumentType       *string  `json:"document_type,omitzero"`
	OCRStatus          string   `json:"ocr_status"`
	OCRConfidence      *float64 `json:"ocr_confidence,omitzero"`
	VerificationStatus string   `json:"verification_status"`
	CreatedAt          string   `json:"created_at"`
	VerifiedAt         *string  `json:"verified_at,omitzero"`
	VerifiedBy         *string  `json:"verified_by,omitzero"`
}
