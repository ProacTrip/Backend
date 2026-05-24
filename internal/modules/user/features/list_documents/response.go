// DTO de respuesta para GET /v1/user/profile/documents — lista de documentos.
// Alineado con USER_API.md: 7 campos por documento, document_type y ocr_confidence son nullables.
package list_documents

// DocumentListItemResponse representa un documento en la respuesta de listado.
// Nota: NO incluye storage_key, extracted_data, document_type_id ni metadatos internos.
type DocumentListItemResponse struct {
	ID                 string   `json:"id"`
	FileName           string   `json:"file_name"`
	DocumentType       *string  `json:"document_type,omitzero"`
	OCRStatus          string   `json:"ocr_status"`
	OCRConfidence      *float64 `json:"ocr_confidence,omitzero"`
	VerificationStatus string   `json:"verification_status"`
	CreatedAt          string   `json:"created_at"`
}
