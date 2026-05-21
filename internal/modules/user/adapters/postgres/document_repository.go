// Adapter PostgreSQL para documentos de usuario.
// Implementa domain.DocumentRepository.
// Alineado con migración user_documents y document_types.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Compile-time interface check
var _ domain.DocumentRepository = (*DocumentRepository)(nil)

// DocumentRepository implementa domain.DocumentRepository usando PostgreSQL.
type DocumentRepository struct {
	db *pgxpool.Pool
}

// NewDocumentRepository crea una nueva instancia del repositorio de documentos.
func NewDocumentRepository(db *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{db: db}
}

// =============================================================================
// Create — Inserta un nuevo documento
// =============================================================================

// Create inserta un nuevo documento de usuario.
func (r *DocumentRepository) Create(ctx context.Context, doc *domain.UserDocument) error {
	query := `
		INSERT INTO user_documents (
			id, user_id, document_type_id, file_name, file_size,
			mime_type, storage_key, detected_mime_type, detected_size_bytes,
			document_type, failure_reason,
			verification_status,
			ocr_status, ocr_data, ocr_confidence, extracted_data,
			has_newer_medical_data, medical_update_summary,
			valid_from, valid_until, document_number, issuing_country,
			metadata, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11,
			$12,
			$13, $14, $15, $16,
			$17, $18,
			$19, $20, $21, $22,
			$23, $24, $25
		)
	`

	_, err := r.db.Exec(ctx, query,
		doc.ID,
		doc.UserID,
		doc.DocumentTypeID,
		doc.FileName,
		doc.FileSize,
		doc.MimeType,
		doc.StorageKey,
		doc.DetectedMimeType,
		doc.DetectedSizeBytes,
		doc.DocumentType,
		doc.FailureReason,
		doc.VerificationStatus,
		doc.OCRStatus,
		doc.OCRData,
		doc.OCRConfidence,
		doc.ExtractedData,
		doc.HasNewerMedicalData,
		doc.MedicalUpdateSummary,
		doc.ValidFrom,
		doc.ValidUntil,
		doc.DocumentNumber,
		doc.IssuingCountry,
		doc.Metadata,
		doc.CreatedAt,
		doc.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create document: %w", err)
	}

	return nil
}

// =============================================================================
// GetByID — Recupera un documento por su ID
// =============================================================================

// GetByID recupera un documento por su ID.
// Retorna ErrDocumentNotFound si no existe.
func (r *DocumentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	query := `
		SELECT
			id, user_id, document_type_id, file_name, file_size,
			mime_type, storage_key, detected_mime_type, detected_size_bytes,
			document_type, failure_reason,
			verification_status,
			ocr_status, ocr_data, ocr_confidence, extracted_data,
			has_newer_medical_data, medical_update_summary,
			valid_from, valid_until, document_number, issuing_country,
			metadata, created_at, updated_at
		FROM user_documents
		WHERE id = $1
	`

	var doc domain.UserDocument

	err := r.db.QueryRow(ctx, query, id).Scan(
		&doc.ID,
		&doc.UserID,
		&doc.DocumentTypeID,
		&doc.FileName,
		&doc.FileSize,
		&doc.MimeType,
		&doc.StorageKey,
		&doc.DetectedMimeType,
		&doc.DetectedSizeBytes,
		&doc.DocumentType,
		&doc.FailureReason,
		&doc.VerificationStatus,
		&doc.OCRStatus,
		&doc.OCRData,
		&doc.OCRConfidence,
		&doc.ExtractedData,
		&doc.HasNewerMedicalData,
		&doc.MedicalUpdateSummary,
		&doc.ValidFrom,
		&doc.ValidUntil,
		&doc.DocumentNumber,
		&doc.IssuingCountry,
		&doc.Metadata,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrDocumentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query document by id: %w", err)
	}

	return &doc, nil
}

// =============================================================================
// GetByUserID — Lista los documentos de un usuario
// =============================================================================

// documentListQuery construye la consulta base de listado de documentos.
const documentListQuery = `
	SELECT
		id, user_id, document_type_id, file_name, file_size,
		mime_type, storage_key, detected_mime_type, detected_size_bytes,
		document_type, failure_reason,
		verification_status,
		ocr_status, ocr_data, ocr_confidence, extracted_data,
		has_newer_medical_data, medical_update_summary,
		valid_from, valid_until, document_number, issuing_country,
		metadata, created_at, updated_at
	FROM user_documents
`

// GetByUserID recupera todos los documentos de un usuario.
func (r *DocumentRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error) {
	query := documentListQuery + ` WHERE user_id = $1 ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get documents by user_id: %w", err)
	}
	defer rows.Close()

	return scanDocuments(rows)
}

// CountByUserID retorna la cantidad de documentos que tiene un usuario.
func (r *DocumentRepository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_documents WHERE user_id = $1`, userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count documents by user_id: %w", err)
	}
	return count, nil
}

// GetByUserIDFiltered recupera los documentos de un usuario con filtros opcionales.
func (r *DocumentRepository) GetByUserIDFiltered(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docTypeCode string) ([]*domain.UserDocument, error) {
	query := documentListQuery + ` WHERE user_id = $1`
	args := []interface{}{userID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(` AND ocr_status = $%d`, argIdx)
		args = append(args, string(status))
		argIdx++
	}
	if docTypeCode != "" {
		query += fmt.Sprintf(` AND document_type = $%d`, argIdx)
		args = append(args, docTypeCode)
		argIdx++
	}

	query += ` ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get documents by user_id filtered: %w", err)
	}
	defer rows.Close()

	return scanDocuments(rows)
}

// scanDocuments escanea las filas de user_documents a un slice de UserDocument.
func scanDocuments(rows pgx.Rows) ([]*domain.UserDocument, error) {
	var docs []*domain.UserDocument

	for rows.Next() {
		var doc domain.UserDocument

		err := rows.Scan(
			&doc.ID,
			&doc.UserID,
			&doc.DocumentTypeID,
			&doc.FileName,
			&doc.FileSize,
			&doc.MimeType,
			&doc.StorageKey,
			&doc.DetectedMimeType,
			&doc.DetectedSizeBytes,
			&doc.DocumentType,
			&doc.FailureReason,
			&doc.VerificationStatus,
			&doc.OCRStatus,
			&doc.OCRData,
			&doc.ExtractedData,
			&doc.HasNewerMedicalData,
			&doc.MedicalUpdateSummary,
			&doc.ValidFrom,
			&doc.ValidUntil,
			&doc.DocumentNumber,
			&doc.IssuingCountry,
			&doc.Metadata,
			&doc.CreatedAt,
			&doc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan document row: %w", err)
		}

		docs = append(docs, &doc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("document rows iteration: %w", err)
	}

	return docs, nil
}

// =============================================================================
// Update — Actualiza un documento
// =============================================================================

// Update persiste los cambios de un documento.
func (r *DocumentRepository) Update(ctx context.Context, doc *domain.UserDocument) error {
	query := `
		UPDATE user_documents SET
			document_type_id = $2,
			file_name = $3,
			file_size = $4,
			mime_type = $5,
			storage_key = $6,
			detected_mime_type = $7,
			detected_size_bytes = $8,
			document_type = $9,
			failure_reason = $10,
			verification_status = $11,
			ocr_status = $12,
			ocr_data = $13,
			ocr_confidence = $14,
			extracted_data = $15,
			has_newer_medical_data = $16,
			medical_update_summary = $17,
			valid_from = $18,
			valid_until = $19,
			document_number = $20,
			issuing_country = $21,
			metadata = $22,
			updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query,
		doc.ID,
		doc.DocumentTypeID,
		doc.FileName,
		doc.FileSize,
		doc.MimeType,
		doc.StorageKey,
		doc.DetectedMimeType,
		doc.DetectedSizeBytes,
		doc.DocumentType,
		doc.FailureReason,
		doc.VerificationStatus,
		doc.OCRStatus,
		doc.OCRData,
		doc.OCRConfidence,
		doc.ExtractedData,
		doc.HasNewerMedicalData,
		doc.MedicalUpdateSummary,
		doc.ValidFrom,
		doc.ValidUntil,
		doc.DocumentNumber,
		doc.IssuingCountry,
		doc.Metadata,
	)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrDocumentNotFound
	}

	return nil
}

// =============================================================================
// Delete — Elimina un documento
// =============================================================================

// Delete elimina un documento por su ID.
func (r *DocumentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, `DELETE FROM user_documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}

// =============================================================================
// GetTypes — Catálogo de tipos de documento (seed data)
// =============================================================================

// GetTypes retorna todos los tipos de documento activos del catálogo.
func (r *DocumentRepository) GetTypes(ctx context.Context) ([]domain.DocumentType, error) {
	query := `
		SELECT
			id, code, name, description, is_identity,
			requires_ocr, ocr_fields, is_active, sort_order, created_at
		FROM document_types
		WHERE is_active = true
		ORDER BY sort_order ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get document types: %w", err)
	}
	defer rows.Close()

	var types []domain.DocumentType
	for rows.Next() {
		var dt domain.DocumentType
		err := rows.Scan(
			&dt.ID,
			&dt.Code,
			&dt.Name,
			&dt.Description,
			&dt.IsIdentity,
			&dt.RequiresOCR,
			&dt.OCRFields,
			&dt.IsActive,
			&dt.SortOrder,
			&dt.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan document type row: %w", err)
		}
		types = append(types, dt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("document types rows iteration: %w", err)
	}

	return types, nil
}

// Ensure json import is used
var _ = json.Marshal
