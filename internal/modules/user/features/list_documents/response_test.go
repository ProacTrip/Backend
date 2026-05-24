// Tests para DocumentListItemResponse DTO — alineado con USER_API.md.
package list_documents

import (
	"encoding/json"
	"testing"
)

// =============================================================================
// T-1.3: DocumentListItemResponse DTO
// =============================================================================

func TestDocumentListItemResponse_AllFields(t *testing.T) {
	confidence := 0.97
	docType := "passport"
	dto := DocumentListItemResponse{
		ID:                 "019d5439-cb43-716d-90b5-51dcbe980908",
		FileName:           "pasaporte.pdf",
		DocumentType:       &docType,
		OCRStatus:          "completed",
		OCRConfidence:      &confidence,
		VerificationStatus: "verified",
		CreatedAt:          "2026-05-01T10:30:00Z",
	}

	if dto.ID == "" {
		t.Error("ID no debería estar vacío")
	}
	if dto.FileName != "pasaporte.pdf" {
		t.Errorf("FileName = %s, se esperaba pasaporte.pdf", dto.FileName)
	}
	if dto.DocumentType == nil || *dto.DocumentType != "passport" {
		t.Error("DocumentType debería ser passport")
	}
	if dto.OCRStatus != "completed" {
		t.Errorf("OCRStatus = %s, se esperaba completed", dto.OCRStatus)
	}
	if dto.OCRConfidence == nil || *dto.OCRConfidence != 0.97 {
		t.Error("OCRConfidence debería ser 0.97")
	}
	if dto.VerificationStatus != "verified" {
		t.Errorf("VerificationStatus = %s, se esperaba verified", dto.VerificationStatus)
	}
	if dto.CreatedAt != "2026-05-01T10:30:00Z" {
		t.Errorf("CreatedAt = %s, se esperaba 2026-05-01T10:30:00Z", dto.CreatedAt)
	}
}

func TestDocumentListItemResponse_JSON_Shape(t *testing.T) {
	confidence := 0.97
	docType := "passport"
	dto := DocumentListItemResponse{
		ID:                 "019d5439-cb43-716d-90b5-51dcbe980908",
		FileName:           "pasaporte.pdf",
		DocumentType:       &docType,
		OCRStatus:          "completed",
		OCRConfidence:      &confidence,
		VerificationStatus: "verified",
		CreatedAt:          "2026-05-01T10:30:00Z",
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Deben existir exactamente 7 campos (document_type *string no nulo, así que aparece)
	requiredFields := []string{"id", "file_name", "document_type", "ocr_status", "ocr_confidence", "verification_status", "created_at"}
	for _, field := range requiredFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("campo requerido %q no está presente en JSON", field)
		}
	}
	if decoded["id"] != "019d5439-cb43-716d-90b5-51dcbe980908" {
		t.Errorf("id = %v, mismatch", decoded["id"])
	}
	if decoded["file_name"] != "pasaporte.pdf" {
		t.Errorf("file_name = %v, mismatch", decoded["file_name"])
	}
	if decoded["ocr_status"] != "completed" {
		t.Errorf("ocr_status = %v, se esperaba completed", decoded["ocr_status"])
	}
	if decoded["verification_status"] != "verified" {
		t.Errorf("verification_status = %v, se esperaba verified", decoded["verification_status"])
	}
	if decoded["created_at"] != "2026-05-01T10:30:00Z" {
		t.Errorf("created_at = %v, mismatch", decoded["created_at"])
	}
}

func TestDocumentListItemResponse_NullableFields(t *testing.T) {
	// Cuando document_type y ocr_confidence son nil, deben omitirse
	dto := DocumentListItemResponse{
		ID:                 "019d5439-cb43-716d-90b5-51dcbe980909",
		FileName:           "seguro_viaje.pdf",
		DocumentType:       nil,
		OCRStatus:          "processing",
		OCRConfidence:      nil,
		VerificationStatus: "unverified",
		CreatedAt:          "2026-05-02T14:22:00Z",
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// nil fields should be omitted
	if _, exists := decoded["document_type"]; exists {
		t.Error("document_type nil debería omitirse con omitzero")
	}
	if _, exists := decoded["ocr_confidence"]; exists {
		t.Error("ocr_confidence nil debería omitirse con omitzero")
	}

	// Required fields should still be present
	if decoded["file_name"] != "seguro_viaje.pdf" {
		t.Errorf("file_name = %v, se esperaba seguro_viaje.pdf", decoded["file_name"])
	}
	if decoded["ocr_status"] != "processing" {
		t.Errorf("ocr_status = %v, se esperaba processing", decoded["ocr_status"])
	}
}
