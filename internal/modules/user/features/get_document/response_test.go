// Tests para DocumentDetailResponse DTO — alineado con USER_API.md.
package get_document

import (
	"encoding/json"
	"testing"
)

// =============================================================================
// T-1.4: DocumentDetailResponse DTO
// =============================================================================

func TestDocumentDetailResponse_Struct(t *testing.T) {
	confidence := 0.97
	verifiedAt := "2026-05-10T08:00:00Z"
	verifiedBy := "admin@proactrip.com"
	docType := "passport"

	dto := DocumentDetailResponse{
		ID:                 "019d5439-cb43-716d-90b5-51dcbe980908",
		FileName:           "pasaporte.pdf",
		DocumentType:       &docType,
		OCRStatus:          "completed",
		OCRConfidence:      &confidence,
		VerificationStatus: "verified",
		CreatedAt:          "2026-05-01T10:30:00Z",
		VerifiedAt:         &verifiedAt,
		VerifiedBy:         &verifiedBy,
	}

	if dto.ID == "" {
		t.Error("ID no debería estar vacío")
	}
	if dto.FileName != "pasaporte.pdf" {
		t.Errorf("FileName = %s, se esperaba pasaporte.pdf", dto.FileName)
	}
	if dto.VerifiedAt == nil || *dto.VerifiedAt != "2026-05-10T08:00:00Z" {
		t.Error("VerifiedAt debería estar presente")
	}
	if dto.VerifiedBy == nil || *dto.VerifiedBy != "admin@proactrip.com" {
		t.Error("VerifiedBy debería estar presente")
	}
	if *dto.DocumentType != "passport" {
		t.Errorf("DocumentType = %s, se esperaba passport", *dto.DocumentType)
	}
}

func TestDocumentDetailResponse_JSON_Shape(t *testing.T) {
	confidence := 0.97
	verifiedAt := "2026-05-10T08:00:00Z"
	verifiedBy := "admin@proactrip.com"
	docType := "passport"

	dto := DocumentDetailResponse{
		ID:                 "019d5439-cb43-716d-90b5-51dcbe980908",
		FileName:           "pasaporte.pdf",
		DocumentType:       &docType,
		OCRStatus:          "completed",
		OCRConfidence:      &confidence,
		VerificationStatus: "verified",
		CreatedAt:          "2026-05-01T10:30:00Z",
		VerifiedAt:         &verifiedAt,
		VerifiedBy:         &verifiedBy,
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Required fields from list + detail-specific fields
	requiredFields := []string{"id", "file_name", "document_type", "ocr_status", "ocr_confidence",
		"verification_status", "created_at", "verified_at", "verified_by"}
	for _, field := range requiredFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("campo requerido %q no está presente en JSON", field)
		}
	}
	if decoded["verified_at"] != "2026-05-10T08:00:00Z" {
		t.Errorf("verified_at = %v, mismatch", decoded["verified_at"])
	}
	if decoded["verified_by"] != "admin@proactrip.com" {
		t.Errorf("verified_by = %v, mismatch", decoded["verified_by"])
	}
}

func TestDocumentDetailResponse_NullableVerified(t *testing.T) {
	docType := "passport"
	dto := DocumentDetailResponse{
		ID:                 "019d5439",
		FileName:           "doc.pdf",
		DocumentType:       &docType,
		OCRStatus:          "processing",
		VerificationStatus: "unverified",
		CreatedAt:          "2026-05-01T10:30:00Z",
		VerifiedAt:         nil,
		VerifiedBy:         nil,
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// nil verified fields should be omitted
	if _, exists := decoded["verified_at"]; exists {
		t.Error("verified_at nil debería omitirse con omitzero")
	}
	if _, exists := decoded["verified_by"]; exists {
		t.Error("verified_by nil debería omitirse con omitzero")
	}
}
