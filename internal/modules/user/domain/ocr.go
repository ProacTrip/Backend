// Dominio de OCR — puerto de extracción de datos de documentos.
// Define la interfaz OCRService y el struct ExtractedData.
package domain

import (
	"context"
)

// =============================================================================
// OCRService — Puerto para extracción de datos de documentos vía OCR/AI
// =============================================================================

// OCRService es el puerto para extraer datos de documentos de viaje.
// Las implementaciones se adaptan a backends específicos (DeepSeek V4 Flash, etc.).
type OCRService interface {
	ExtractFromDocument(ctx context.Context, fileURL string) (*ExtractedData, error)
}

// =============================================================================
// ExtractedData — Datos extraídos de un documento
// =============================================================================

// ExtractedData contiene los datos extraídos por OCR/AI de un documento.
type ExtractedData struct {
	// Identificación del documento
	DocumentType   string  `json:"document_type"`   // passport, national_id, visa, vaccination_cert, etc.
	DocumentNumber *string `json:"document_number,omitzero"`
	FullName       *string `json:"full_name,omitzero"`
	DateOfBirth    *string `json:"date_of_birth,omitzero"`
	ExpiryDate     *string `json:"expiry_date,omitzero"`
	IssuingCountry *string `json:"issuing_country,omitzero"`
	Nationality    *string `json:"nationality,omitzero"`
	Gender         *string `json:"gender,omitzero"` // M, F, X — extraído de documentos de identidad

	// Campos médicos (extraídos de certificados, recetas, etc.)
	MedicalFields map[string]string `json:"medical_fields,omitzero"`

	// Metadatos del OCR
	OCRConfidence float64 `json:"ocr_confidence"`
	RawResponse   string  `json:"raw_response,omitzero"`
}

// IsTravelDocument retorna true si el documento extraído es de viaje reconocido.
func (e *ExtractedData) IsTravelDocument() bool {
	switch e.DocumentType {
	case "passport", "national_id", "drivers_license", "visa", "vaccination_cert", "travel_insurance":
		return true
	default:
		return false
	}
}

// IsIdentityDocument retorna true si es documento de identidad (pasaporte).
func (e *ExtractedData) IsIdentityDocument() bool {
	return e.DocumentType == "passport"
}
