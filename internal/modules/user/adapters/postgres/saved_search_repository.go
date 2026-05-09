// Adapter PostgreSQL para búsquedas guardadas.
// Implementa domain.SavedSearchRepository.
// Alineado con migración saved_searches.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Compile-time interface check
var _ domain.SavedSearchRepository = (*SavedSearchRepository)(nil)

// SavedSearchRepository implementa domain.SavedSearchRepository usando PostgreSQL.
type SavedSearchRepository struct {
	db *pgxpool.Pool
}

// NewSavedSearchRepository crea una nueva instancia del repositorio.
func NewSavedSearchRepository(db *pgxpool.Pool) *SavedSearchRepository {
	return &SavedSearchRepository{db: db}
}

// =============================================================================
// Create — Inserta una nueva búsqueda guardada
// =============================================================================

func (r *SavedSearchRepository) Create(ctx context.Context, search *domain.SavedSearch) error {
	query := `
		INSERT INTO saved_searches (
			id, user_id, name, parameters, filters, search_hash,
			search_type, parameters_version, alert_enabled, last_executed_at, result_count,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := r.db.Exec(ctx, query,
		search.ID,
		search.UserID,
		search.Name,
		search.Parameters,
		search.Filters,
		search.SearchHash,
		search.SearchType,
		search.ParametersVersion,
		search.AlertEnabled,
		search.LastExecutedAt,
		search.ResultCount,
		search.CreatedAt,
		search.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create saved search: %w", err)
	}
	return nil
}

// =============================================================================
// GetByUserID — Lista todas las búsquedas de un usuario
// =============================================================================

func (r *SavedSearchRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error) {
	query := `
		SELECT id, user_id, name, parameters, filters, search_hash,
		       search_type, parameters_version, alert_enabled, last_executed_at, result_count,
		       created_at, updated_at
		FROM saved_searches
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get saved searches by user_id: %w", err)
	}
	defer rows.Close()

	var searches []*domain.SavedSearch
	for rows.Next() {
		var s domain.SavedSearch
		if err := rows.Scan(
			&s.ID,
			&s.UserID,
			&s.Name,
			&s.Parameters,
			&s.Filters,
			&s.SearchHash,
			&s.SearchType,
			&s.ParametersVersion,
			&s.AlertEnabled,
			&s.LastExecutedAt,
			&s.ResultCount,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan saved search: %w", err)
		}
		searches = append(searches, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return searches, nil
}

// =============================================================================
// GetByHash — Búsqueda por hash para deduplicación por usuario
// =============================================================================

func (r *SavedSearchRepository) GetByHash(ctx context.Context, userID uuid.UUID, searchHash string) (*domain.SavedSearch, error) {
	query := `
		SELECT id, user_id, name, parameters, filters, search_hash,
		       search_type, parameters_version, alert_enabled, last_executed_at, result_count,
		       created_at, updated_at
		FROM saved_searches
		WHERE user_id = $1 AND search_hash = $2
	`

	var s domain.SavedSearch
	err := r.db.QueryRow(ctx, query, userID, searchHash).Scan(
		&s.ID,
		&s.UserID,
		&s.Name,
		&s.Parameters,
		&s.Filters,
		&s.SearchHash,
		&s.SearchType,
		&s.ParametersVersion,
		&s.AlertEnabled,
		&s.LastExecutedAt,
		&s.ResultCount,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // No encontrada, no es error
	}
	if err != nil {
		return nil, fmt.Errorf("get saved search by hash: %w", err)
	}
	return &s, nil
}

// =============================================================================
// GetByID — Búsqueda por ID (para verificar ownership)
// =============================================================================

func (r *SavedSearchRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
	query := `
		SELECT id, user_id, name, parameters, filters, search_hash,
		       search_type, parameters_version, alert_enabled, last_executed_at, result_count,
		       created_at, updated_at
		FROM saved_searches
		WHERE id = $1
	`

	var s domain.SavedSearch
	err := r.db.QueryRow(ctx, query, id).Scan(
		&s.ID,
		&s.UserID,
		&s.Name,
		&s.Parameters,
		&s.Filters,
		&s.SearchHash,
		&s.SearchType,
		&s.ParametersVersion,
		&s.AlertEnabled,
		&s.LastExecutedAt,
		&s.ResultCount,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrSearchNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get saved search by id: %w", err)
	}
	return &s, nil
}

// =============================================================================
// Update — Actualización parcial de nombre, parámetros y filtros
// =============================================================================

func (r *SavedSearchRepository) Update(ctx context.Context, search *domain.SavedSearch) error {
	query := `
		UPDATE saved_searches SET
			name        = COALESCE($2, name),
			parameters  = COALESCE($3, parameters),
			filters     = COALESCE($4, filters),
			search_hash = COALESCE(NULLIF($5, ''), search_hash),
			search_type = COALESCE($6, search_type),
			updated_at  = NOW()
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query,
		search.ID,
		search.Name,
		search.Parameters,
		search.Filters,
		search.SearchHash,
		search.SearchType,
	)
	if err != nil {
		return fmt.Errorf("update saved search: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrSearchNotFound
	}
	return nil
}

// =============================================================================
// Delete — Elimina una búsqueda guardada
// =============================================================================

func (r *SavedSearchRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM saved_searches WHERE id = $1`

	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete saved search: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrSearchNotFound
	}
	return nil
}

// =============================================================================
// SetAlertEnabled — Establece el estado de alerta
// =============================================================================

func (r *SavedSearchRepository) SetAlertEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	query := `
		UPDATE saved_searches SET
			alert_enabled = $2,
			updated_at    = NOW()
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, id, enabled)
	if err != nil {
		return fmt.Errorf("set alert enabled: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrSearchNotFound
	}
	return nil
}
