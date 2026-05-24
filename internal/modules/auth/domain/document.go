package domain

import (
	"time"

	"github.com/google/uuid"
)

// DocumentRow representa una fila de la tabla user_documents.
// Tipo de dominio compartido entre features del dashboard y el adapter postgres.
// La tabla user_documents está en el schema del módulo user; el adapter del módulo
// auth la consulta directamente via pgx (cross-schema, mismo cluster DB).
type DocumentRow struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	VerificationStatus string
	OCRStatus          string
	DocumentTypeID     uuid.UUID
	DocumentTypeCode   string
	FileName           string
	FileSize           *int
	MimeType           string
	StorageKey         string
	OCRConfidence      *float64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// DocumentSummary es un DTO liviano para incluir en la respuesta de User Detail.
// UD-REQ-1: id, file_name, document_type, ocr_status, ocr_confidence,
// verification_status, file_size, created_at.
type DocumentSummary struct {
	ID                 uuid.UUID  `json:"id"`
	FileName           string     `json:"file_name"`
	DocumentType       string     `json:"document_type"`
	OCRStatus          string     `json:"ocr_status"`
	OCRConfidence      *float64   `json:"ocr_confidence,omitzero"`
	VerificationStatus string     `json:"verification_status"`
	FileSize           *int       `json:"file_size,omitzero"`
	CreatedAt          time.Time  `json:"created_at"`
}

// HistoryEntry representa una entrada del historial de verificación (append-only).
type HistoryEntry struct {
	ID             uuid.UUID
	DocumentID     uuid.UUID
	PreviousStatus string
	NewStatus      string
	VerifiedBy     uuid.UUID
	Reason         *string
	ChangedAt      time.Time
}
