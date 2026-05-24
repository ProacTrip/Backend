// Adaptador PostgreSQL para queries de documentos del dashboard.
// Consulta la tabla user_documents del módulo user directamente via pgx
// (cross-schema, mismo cluster DB — sigue el patrón de dashboard_repository.go).
//
// Las interfaces que implementa están definidas en los features del dashboard:
//   - document_verification/usecase.go: DocumentVerificationRepo
//   - document_reprocess/usecase.go: DocumentReprocessRepo
//   - user_detail/usecase.go: DocumentLister
//
// La dependencia postgres→features es válida: las features no importan postgres
// (solo definen interfaces que el adapter satisface).
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// DocumentRepository — adapter que consulta user_documents (schema user)
// =============================================================================

// DocumentRepository implementa las interfaces de documentos del dashboard.
// Usa el mismo pool pgx que el resto del módulo auth.
type DocumentRepository struct {
	pool PgxPool
}

// NewDocumentRepository crea un nuevo repositorio de documentos.
func NewDocumentRepository(pool PgxPool) *DocumentRepository {
	return &DocumentRepository{pool: pool}
}

// =============================================================================
// GetByID — consulta básica de un documento
// =============================================================================

// GetByID obtiene un documento por su ID desde la tabla user_documents.
// Hace JOIN con document_types para obtener el código del tipo de documento.
// Retorna ErrDocumentNotFound si no existe.
func (r *DocumentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
	query := `
		SELECT ud.id, ud.user_id, ud.verification_status, ud.ocr_status,
		       ud.document_type_id, dt.code AS document_type_code,
		       ud.file_name, ud.file_size, ud.mime_type, ud.storage_key,
		       ud.ocr_confidence, ud.created_at, ud.updated_at
		FROM user_documents ud
		JOIN document_types dt ON ud.document_type_id = dt.id
		WHERE ud.id = $1
	`

	var row domain.DocumentRow
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&row.ID,
		&row.UserID,
		&row.VerificationStatus,
		&row.OCRStatus,
		&row.DocumentTypeID,
		&row.DocumentTypeCode,
		&row.FileName,
		&row.FileSize,
		&row.MimeType,
		&row.StorageKey,
		&row.OCRConfidence,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDocumentNotFound
		}
		return nil, fmt.Errorf("get document by id: %w", err)
	}

	return &row, nil
}

// =============================================================================
// GetByIDForUpdate — consulta con row-level lock (SELECT ... FOR UPDATE)
// =============================================================================

// GetByIDForUpdate bloquea la fila para evitar race conditions en PATCH de verificación.
// Usa SELECT ... FOR UPDATE para prevenir que dos admins modifiquen el mismo estado
// simultáneamente. El lock se libera al hacer COMMIT del transaction block.
func (r *DocumentRepository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error) {
	query := `
		SELECT ud.id, ud.user_id, ud.verification_status, ud.ocr_status,
		       ud.document_type_id, dt.code AS document_type_code,
		       ud.file_name, ud.file_size, ud.mime_type, ud.storage_key,
		       ud.ocr_confidence, ud.created_at, ud.updated_at
		FROM user_documents ud
		JOIN document_types dt ON ud.document_type_id = dt.id
		WHERE ud.id = $1
		FOR UPDATE
	`

	var row domain.DocumentRow
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&row.ID,
		&row.UserID,
		&row.VerificationStatus,
		&row.OCRStatus,
		&row.DocumentTypeID,
		&row.DocumentTypeCode,
		&row.FileName,
		&row.FileSize,
		&row.MimeType,
		&row.StorageKey,
		&row.OCRConfidence,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDocumentNotFound
		}
		return nil, fmt.Errorf("get document for update: %w", err)
	}

	return &row, nil
}

// =============================================================================
// GetHistory — historial de verificación (append-only)
// =============================================================================

// GetHistory obtiene todas las entradas del historial de verificación
// para un documento, ordenadas por changed_at DESC (más reciente primero).
// DV-REQ-1: incluido en la respuesta de verificación.
func (r *DocumentRepository) GetHistory(ctx context.Context, documentID uuid.UUID) ([]domain.HistoryEntry, error) {
	query := `
		SELECT id, document_id, previous_status, new_status,
		       verified_by, reason, changed_at
		FROM document_verification_history
		WHERE document_id = $1
		ORDER BY changed_at DESC
	`

	rows, err := r.pool.Query(ctx, query, documentID)
	if err != nil {
		return nil, fmt.Errorf("get document history: %w", err)
	}
	defer rows.Close()

	var entries []domain.HistoryEntry
	for rows.Next() {
		var e domain.HistoryEntry
		if scanErr := rows.Scan(
			&e.ID,
			&e.DocumentID,
			&e.PreviousStatus,
			&e.NewStatus,
			&e.VerifiedBy,
			&e.Reason,
			&e.ChangedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan history entry: %w", scanErr)
		}
		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history entries: %w", err)
	}

	return entries, nil
}

// =============================================================================
// InsertHistory — append-only row en document_verification_history
// =============================================================================

// InsertHistory inserta una nueva entrada en el historial de verificación.
// DV-REQ-3: el historial es append-only (no hay UPDATE ni DELETE).
func (r *DocumentRepository) InsertHistory(ctx context.Context, entry domain.HistoryEntry) error {
	query := `
		INSERT INTO document_verification_history
			(document_id, previous_status, new_status, verified_by, reason)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.pool.Exec(ctx, query,
		entry.DocumentID,
		entry.PreviousStatus,
		entry.NewStatus,
		entry.VerifiedBy,
		entry.Reason,
	)
	if err != nil {
		return fmt.Errorf("insert history entry: %w", err)
	}

	return nil
}

// =============================================================================
// UpdateVerificationStatus — actualiza el estado de verificación
// =============================================================================

// UpdateVerificationStatus actualiza verification_status en user_documents.
// DV-REQ-2: el status debe ser uno de {verified, rejected, manual_review, suspicious}.
// La validación del status la hace el usecase antes de llamar este método.
func (r *DocumentRepository) UpdateVerificationStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE user_documents SET verification_status = $1, updated_at = NOW() WHERE id = $2`

	ct, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update verification status: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrDocumentNotFound
	}

	return nil
}

// =============================================================================
// UpdateOCRStatus — actualiza el estado de OCR (para reprocess)
// =============================================================================

// UpdateOCRStatus actualiza ocr_status en user_documents.
// DR-REQ-1: usado por el feature document_reprocess para poner el doc en "queued".
func (r *DocumentRepository) UpdateOCRStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE user_documents SET ocr_status = $1, updated_at = NOW() WHERE id = $2`

	ct, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update ocr status: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrDocumentNotFound
	}

	return nil
}

// =============================================================================
// GetUserDocuments — lista documentos de un usuario (para User Detail)
// =============================================================================

// GetUserDocuments obtiene los documentos asociados a un user_id.
// UD-REQ-1: retorna DocumentSummary con los campos requeridos.
// UD-REQ-1.2: si el usuario no tiene documentos, retorna slice vacío (no nil).
func (r *DocumentRepository) GetUserDocuments(ctx context.Context, userID uuid.UUID) ([]domain.DocumentSummary, error) {
	query := `
		SELECT ud.id, ud.file_name, dt.code AS document_type,
		       ud.ocr_status, ud.ocr_confidence,
		       ud.verification_status, ud.file_size, ud.created_at
		FROM user_documents ud
		JOIN document_types dt ON ud.document_type_id = dt.id
		WHERE ud.user_id = $1
		ORDER BY ud.created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get user documents: %w", err)
	}
	defer rows.Close()

	docs := make([]domain.DocumentSummary, 0)
	for rows.Next() {
		var d domain.DocumentSummary
		if scanErr := rows.Scan(
			&d.ID,
			&d.FileName,
			&d.DocumentType,
			&d.OCRStatus,
			&d.OCRConfidence,
			&d.VerificationStatus,
			&d.FileSize,
			&d.CreatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan document summary: %w", scanErr)
		}
		docs = append(docs, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user documents: %w", err)
	}

	return docs, nil
}

// Compile-time check: DocumentRepository implementa las interfaces requeridas.
var _ interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
	GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
	GetHistory(ctx context.Context, documentID uuid.UUID) ([]domain.HistoryEntry, error)
	InsertHistory(ctx context.Context, entry domain.HistoryEntry) error
	UpdateVerificationStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdateOCRStatus(ctx context.Context, id uuid.UUID, status string) error
	GetUserDocuments(ctx context.Context, userID uuid.UUID) ([]domain.DocumentSummary, error)
} = (*DocumentRepository)(nil)
