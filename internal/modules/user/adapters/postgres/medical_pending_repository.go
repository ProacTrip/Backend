// Adapter PostgreSQL para actualizaciones médicas pendientes.
// Implementa domain.MedicalPendingUpdateRepository.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// MedicalPendingUpdateRepository — PostgreSQL adapter
// Alineado con migración medical_pending_updates
// =============================================================================

// Compile-time interface check
var _ domain.MedicalPendingUpdateRepository = (*MedicalPendingUpdateRepository)(nil)

// MedicalPendingUpdateRepository implementa domain.MedicalPendingUpdateRepository.
type MedicalPendingUpdateRepository struct {
	db *pgxpool.Pool
}

// NewMedicalPendingUpdateRepository crea una nueva instancia del repositorio.
func NewMedicalPendingUpdateRepository(db *pgxpool.Pool) *MedicalPendingUpdateRepository {
	return &MedicalPendingUpdateRepository{db: db}
}

// =============================================================================
// Create — Inserta una nueva actualización pendiente
// =============================================================================

// Create inserta una nueva actualización médica pendiente.
func (r *MedicalPendingUpdateRepository) Create(ctx context.Context, update *domain.MedicalPendingUpdate) error {
	query := `
		INSERT INTO medical_pending_updates (
			id, user_id, source_type, source_document_id, conversation_id,
			field_name, current_value, proposed_value,
			suggested_at, expires_at, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err := r.db.Exec(ctx, query,
		update.ID,
		update.UserID,
		update.SourceType,
		update.SourceDocumentID,
		update.ConversationID,
		update.FieldName,
		update.CurrentValue,
		update.ProposedValue,
		update.SuggestedAt,
		update.ExpiresAt,
		string(update.Status),
	)
	if err != nil {
		return fmt.Errorf("create medical pending update: %w", err)
	}

	return nil
}

// =============================================================================
// GetByUserID — Recupera actualizaciones pendientes (solo 'pending')
// =============================================================================

// GetByUserID recupera todas las actualizaciones pendientes (status='pending')
// para un usuario.
func (r *MedicalPendingUpdateRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.MedicalPendingUpdate, error) {
	query := `
		SELECT id, user_id, source_type, source_document_id, conversation_id,
		       field_name, current_value, proposed_value,
		       suggested_at, expires_at, status, resolved_at
		FROM medical_pending_updates
		WHERE user_id = $1 AND status = 'pending'
		ORDER BY suggested_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get pending updates by user_id: %w", err)
	}
	defer rows.Close()

	return scanMedicalPendingUpdates(rows)
}

// =============================================================================
// GetByID — Recupera una actualización pendiente por su ID
// =============================================================================

// GetByID recupera una actualización pendiente por su ID primario.
func (r *MedicalPendingUpdateRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.MedicalPendingUpdate, error) {
	query := `
		SELECT id, user_id, source_type, source_document_id, conversation_id,
		       field_name, current_value, proposed_value,
		       suggested_at, expires_at, status, resolved_at
		FROM medical_pending_updates
		WHERE id = $1
	`

	var pu domain.MedicalPendingUpdate
	var statusStr string

	err := r.db.QueryRow(ctx, query, id).Scan(
		&pu.ID,
		&pu.UserID,
		&pu.SourceType,
		&pu.SourceDocumentID,
		&pu.ConversationID,
		&pu.FieldName,
		&pu.CurrentValue,
		&pu.ProposedValue,
		&pu.SuggestedAt,
		&pu.ExpiresAt,
		&statusStr,
		&pu.ResolvedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrPendingUpdateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get pending update by id: %w", err)
	}

	pu.Status = domain.MedicalPendingUpdateStatus(statusStr)

	return &pu, nil
}

// =============================================================================
// Resolve — Marca una actualización como aceptada o rechazada
// =============================================================================

// Resolve actualiza el estado de una actualización pendiente y setea resolved_at.
func (r *MedicalPendingUpdateRepository) Resolve(ctx context.Context, id uuid.UUID, status domain.MedicalPendingUpdateStatus) error {
	now := time.Now()

	query := `
		UPDATE medical_pending_updates
		SET status = $2, resolved_at = $3
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, id, string(status), now)
	if err != nil {
		return fmt.Errorf("resolve pending update: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrPendingUpdateNotFound
	}

	return nil
}

// =============================================================================
// CountPending — Cuenta actualizaciones pendientes para un usuario
// =============================================================================

// CountPending retorna la cantidad de actualizaciones pendientes para un usuario.
func (r *MedicalPendingUpdateRepository) CountPending(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM medical_pending_updates
		WHERE user_id = $1 AND status = 'pending'
	`

	var count int
	err := r.db.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending updates: %w", err)
	}

	return count, nil
}

// =============================================================================
// Helpers
// =============================================================================

// scanMedicalPendingUpdates escanea todas las filas de un resultado de query.
func scanMedicalPendingUpdates(rows pgx.Rows) ([]*domain.MedicalPendingUpdate, error) {
	var updates []*domain.MedicalPendingUpdate

	for rows.Next() {
		var pu domain.MedicalPendingUpdate
		var statusStr string

		err := rows.Scan(
			&pu.ID,
			&pu.UserID,
			&pu.SourceType,
			&pu.SourceDocumentID,
			&pu.ConversationID,
			&pu.FieldName,
			&pu.CurrentValue,
			&pu.ProposedValue,
			&pu.SuggestedAt,
			&pu.ExpiresAt,
			&statusStr,
			&pu.ResolvedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan pending update: %w", err)
		}

		pu.Status = domain.MedicalPendingUpdateStatus(statusStr)
		updates = append(updates, &pu)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending updates: %w", err)
	}

	return updates, nil
}
