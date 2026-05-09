// Adapter PostgreSQL para favoritos de usuario.
// Implementa domain.FavoriteRepository.
// Alineado con migración user_favorites (002).
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// isUniqueViolation detecta violaciones de constraint unique (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "23505"
	}
	return false
}

// Compile-time interface check
var _ domain.FavoriteRepository = (*FavoriteRepository)(nil)

// FavoriteRepository implementa domain.FavoriteRepository usando PostgreSQL.
type FavoriteRepository struct {
	db *pgxpool.Pool
}

// NewFavoriteRepository crea una nueva instancia del repositorio de favoritos.
func NewFavoriteRepository(db *pgxpool.Pool) *FavoriteRepository {
	return &FavoriteRepository{db: db}
}

// =============================================================================
// Create — Inserta un nuevo favorito
// =============================================================================

func (r *FavoriteRepository) Create(ctx context.Context, fav *domain.Favorite) error {
	query := `
		INSERT INTO user_favorites (
			id, user_id, entity_id, entity_type, title, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.Exec(ctx, query,
		fav.ID,
		fav.UserID,
		fav.EntityID,
		fav.EntityType,
		fav.Title,
		fav.Notes,
		fav.CreatedAt,
		fav.UpdatedAt,
	)
	if err != nil {
		// Detect unique constraint violation (user_id, entity_id, entity_type)
		if isUniqueViolation(err) {
			return domain.ErrDuplicateFavorite
		}
		return fmt.Errorf("create favorite: %w", err)
	}
	return nil
}

// =============================================================================
// GetByUserID — Lista todos los favoritos del usuario
// =============================================================================

func (r *FavoriteRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Favorite, error) {
	query := `
		SELECT id, user_id, entity_id, entity_type, title, notes, created_at, updated_at
		FROM user_favorites
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get favorites by user_id: %w", err)
	}
	defer rows.Close()

	var favs []*domain.Favorite
	for rows.Next() {
		var f domain.Favorite
		if err := rows.Scan(
			&f.ID,
			&f.UserID,
			&f.EntityID,
			&f.EntityType,
			&f.Title,
			&f.Notes,
			&f.CreatedAt,
			&f.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		favs = append(favs, &f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return favs, nil
}

// =============================================================================
// GetByID — Búsqueda por ID (para verificar ownership)
// =============================================================================

func (r *FavoriteRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Favorite, error) {
	query := `
		SELECT id, user_id, entity_id, entity_type, title, notes, created_at, updated_at
		FROM user_favorites
		WHERE id = $1
	`

	var f domain.Favorite
	err := r.db.QueryRow(ctx, query, id).Scan(
		&f.ID,
		&f.UserID,
		&f.EntityID,
		&f.EntityType,
		&f.Title,
		&f.Notes,
		&f.CreatedAt,
		&f.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrFavoriteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get favorite by id: %w", err)
	}
	return &f, nil
}

// =============================================================================
// Delete — Elimina un favorito
// =============================================================================

func (r *FavoriteRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM user_favorites WHERE id = $1`

	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete favorite: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrFavoriteNotFound
	}
	return nil
}
