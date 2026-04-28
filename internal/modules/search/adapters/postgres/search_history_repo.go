// Repositorio PostgreSQL para guardar historial de búsquedas.
// Implementa domain.SearchHistoryRepository.
package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Interfaz minimal de PgxPool
// =============================================================================

// PgxPool is the minimal pgxpool interface needed by SearchHistoryRepo.
type PgxPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// =============================================================================
// Repositorio de Historial
// =============================================================================

var _ domain.SearchHistoryRepository = (*SearchHistoryRepo)(nil)

// SearchHistoryRepo implements domain.SearchHistoryRepository using PostgreSQL.
type SearchHistoryRepo struct {
	pool PgxPool
}

// NewSearchHistoryRepo creates a new repository backed by a pgx connection pool.
func NewSearchHistoryRepo(pool PgxPool) *SearchHistoryRepo {
	return &SearchHistoryRepo{pool: pool}
}

// Save records a search history entry.
func (r *SearchHistoryRepo) Save(ctx context.Context, entry domain.SearchHistoryEntry) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO search_history 
		 (id, user_id, session_id, query_type, raw_query, parsed_params, result_count, execution_time_ms, cache_hit, ip_address, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		entry.ID,
		toPgUUID(entry.UserID),
		nilIfEmpty(entry.SessionID),
		entry.QueryType,
		entry.RawQuery,
		entry.ParsedParams,
		entry.ResultCount,
		entry.ExecutionTimeMs,
		entry.CacheHit,
		nilIfEmpty(entry.IPAddress),
		nilIfEmpty(entry.UserAgent),
	)
	if err != nil {
		return fmt.Errorf("save search history: %w", err)
	}

	return nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func toPgUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
