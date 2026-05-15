// Adaptador PostgreSQL para las queries específicas del dashboard.
// Extiende UserRepository con métodos de listado, actualización de estado
// y resolución de permisos de rol que el dashboard requiere.
//
// Los métodos implementan interfaces definidas en features/dashboard/ usando
// los tipos de esos paquetes. La dependencia postgres→features es válida:
// las features no importan postgres (solo definen interfaces que el adapter satisface).
package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	listusers "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/list_users"
)

// =============================================================================
// ListUsers — dashboard user listing with filters and pagination
// =============================================================================

// ListUsers implementa list_users.UserListRepo.ListUsers.
// Query con filtros dinámicos y paginación por cursor (offset-based).
// DU-SPEC-001: cursor pagination con ORDER BY created_at DESC, id DESC.
// DU-SPEC-002: filtros combinables: role, status, search, created_before/after.
// DU-SPEC-004: NUNCA selecciona password_hash, locked_until o failed_attempts.
func (r *UserRepository) ListUsers(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if filters.Role != "" {
		conditions = append(conditions, fmt.Sprintf("r.name = $%d", argIdx))
		args = append(args, filters.Role)
		argIdx++
	}
	if filters.Status != "" {
		conditions = append(conditions, fmt.Sprintf("u.status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.Search != "" {
		conditions = append(conditions, fmt.Sprintf("u.email ILIKE $%d", argIdx))
		args = append(args, "%"+filters.Search+"%")
		argIdx++
	}
	if filters.CreatedBefore != "" {
		conditions = append(conditions, fmt.Sprintf("u.created_at <= $%d::timestamptz", argIdx))
		args = append(args, filters.CreatedBefore)
		argIdx++
	}
	if filters.CreatedAfter != "" {
		conditions = append(conditions, fmt.Sprintf("u.created_at >= $%d::timestamptz", argIdx))
		args = append(args, filters.CreatedAfter)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Contar total sin límite/offset
	countQuery := fmt.Sprintf(
		`SELECT COUNT(*) FROM users u JOIN roles r ON u.role_id = r.id %s`,
		whereClause,
	)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	limit := filters.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	dataQuery := fmt.Sprintf(
		`SELECT u.id, u.email, u.status, u.role_id, r.name AS role_name,
		        u.email_verified, u.created_at, u.updated_at
		 FROM users u
		 JOIN roles r ON u.role_id = r.id
		 %s
		 ORDER BY u.created_at DESC, u.id DESC
		 LIMIT $%d OFFSET $%d`,
		whereClause, argIdx, argIdx+1,
	)
	args = append(args, limit, filters.Offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []listusers.UserRow
	for rows.Next() {
		var u listusers.UserRow
		if scanErr := rows.Scan(
			&u.ID, &u.Email, &u.Status, &u.RoleID,
			&u.RoleName, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt,
		); scanErr != nil {
			return nil, 0, fmt.Errorf("scan user row: %w", scanErr)
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate users: %w", err)
	}

	return users, total, nil
}

// =============================================================================
// UpdateStatus — toggle de estado con token_version++ atómico
// =============================================================================

// UpdateStatus implementa account_status.AccountStatusRepo.UpdateStatus.
// AS-SPEC-003: UPDATE users SET status = $1, token_version = token_version + 1,
// updated_at = NOW() WHERE id = $2 RETURNING token_version.
// El incremento de token_version es ATÓMICO (no hay race condition).
func (r *UserRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (int, error) {
	query := `UPDATE users SET status = $1, token_version = token_version + 1,
	          updated_at = NOW() WHERE id = $2 RETURNING token_version`

	var newTV int
	if err := r.pool.QueryRow(ctx, query, status, id).Scan(&newTV); err != nil {
		return 0, fmt.Errorf("update user status: %w", err)
	}

	return newTV, nil
}

// =============================================================================
// GetPermissionsByRoleID — permisos base de un rol
// =============================================================================

// GetPermissionsByRoleID implementa services.RolePermissionRepository.
// Retorna los códigos de permiso asignados a un rol (formato "resource:action").
func (r *UserRepository) GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]string, error) {
	query := `
		SELECT COALESCE(array_agg(p.resource || ':' || p.action) FILTER (WHERE p.resource IS NOT NULL), '{}')
		FROM roles r2
		LEFT JOIN role_permissions rp ON r2.id = rp.role_id
		LEFT JOIN permissions p ON rp.permission_id = p.id
		WHERE r2.id = $1
	`

	var permissions []string
	if err := r.pool.QueryRow(ctx, query, roleID).Scan(&permissions); err != nil {
		return nil, fmt.Errorf("get permissions by role id: %w", err)
	}

	return permissions, nil
}

// Compile-time check: UserRepository implementa las interfaces del dashboard.
var _ interface {
	ListUsers(ctx context.Context, filters listusers.ListUsersFilters) ([]listusers.UserRow, int, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) (int, error)
} = (*UserRepository)(nil)

// Verify UserRepository still satisfies domain.UserRepository + dashboard repos.
var _ domain.UserRepository = (*UserRepository)(nil)
