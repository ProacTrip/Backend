// Lógica de negocio para listado paginado de usuarios del dashboard.
// Orquesta validación, filtros DB y paginación con cursores.
package list_users

import (
	"context"
	"fmt"

	"github.com/ProacTrip/Backend/internal/shared/pagination"
)

// =============================================================================
// Puerto de repositorio — interfaz local que el adapter PG implementa
// =============================================================================

// UserListRepo is the local port for listing users with filters and pagination.
// Implemented by the postgres adapter.
type UserListRepo interface {
	// ListUsers returns users matching filters with total count for pagination.
	// total count is used to compute has_next and cursors.
	ListUsers(ctx context.Context, filters ListUsersFilters) ([]UserRow, int, error)
}

// =============================================================================
// ListUsersFilters — filtros de búsqueda
// =============================================================================

// ListUsersFilters bundles pagination and filter parameters for the DB query.
// Offset is computed from the cursor in the usecase.
type ListUsersFilters struct {
	Offset        int
	Limit         int
	Role          string
	Status        string
	Search        string
	CreatedBefore string
	CreatedAfter  string
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orchestrates user listing with cursor pagination.
type UseCase struct {
	repo UserListRepo
}

// NewUseCase creates a new list users use case.
func NewUseCase(repo UserListRepo) *UseCase {
	return &UseCase{repo: repo}
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute performs the user listing with cursor pagination.
// Flow: validate → decode cursor → build filters → query DB → build meta → respond.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Validate
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Decode cursor to offset
	offset := 0
	if cmd.Cursor != "" {
		o, err := pagination.DecodeCursor(cmd.Cursor)
		if err == nil {
			offset = o
		}
	}

	// 3. Build filters — mapear status público a valor DB
	filters := ListUsersFilters{
		Offset:        offset,
		Limit:         cmd.Limit,
		Role:          cmd.Role,
		Status:        StatusToDB(cmd.Status),
		Search:        cmd.Search,
		CreatedBefore: cmd.CreatedBefore,
		CreatedAfter:  cmd.CreatedAfter,
	}

	// 4. Query DB
	rows, total, err := uc.repo.ListUsers(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	// 5. Build response
	users := make([]UserResponse, len(rows))
	for i, row := range rows {
		users[i] = UserResponse{
			ID:            row.ID,
			Email:         row.Email,
			Status:        row.Status,
			RoleID:        row.RoleID,
			RoleName:      row.RoleName,
			EmailVerified: row.EmailVerified,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
		}
	}

	// 6. Build pagination meta (DU-SPEC-005)
	meta := buildMeta(offset, cmd.Limit, total)

	return &Response{
		Users: users,
		Meta:  meta,
	}, nil
}

// =============================================================================
// Helpers de Paginación
// =============================================================================

// buildMeta constructs Meta following DU-SPEC-005 contract:
// prev_cursor is nil on first page, next_cursor is nil on last page.
func buildMeta(offset, limit, total int) Meta {
	m := Meta{
		HasNext: offset+limit < total,
		Limit:   limit,
	}

	if offset > 0 {
		prev := offset - limit
		if prev < 0 {
			prev = 0
		}
		m.PrevCursor = new(pagination.EncodeCursor(prev))
	}

	if m.HasNext {
		m.NextCursor = new(pagination.EncodeCursor(offset + limit))
	}

	return m
}
