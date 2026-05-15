// Constantes de permisos para el sistema de autorización RBAC.
// Los 9 permisos base que se seedean en la DB (001_initial.sql).
// Usados por RequirePermission middleware y dashboard endpoints.
package auth

// Permisos del módulo dashboard — coinciden con los seeds en permissions table.
// Formato: "resource:action" (ej. "users:read").
const (
	// Lectura y escritura de usuarios del dashboard.
	PermUsersRead  = "users:read"
	PermUsersWrite = "users:write"

	// Lectura y escritura de roles.
	PermRolesRead  = "roles:read"
	PermRolesWrite = "roles:write"

	// Lectura y escritura de permisos RBAC.
	PermPermsRead  = "permissions:read"
	PermPermsWrite = "permissions:write"

	// Lectura y escritura de feature limits.
	PermFeatureLimitsRead  = "feature_limits:read"
	PermFeatureLimitsWrite = "feature_limits:write"

	// Lectura y escritura de sesiones activas.
	PermSessionsRead  = "sessions:read"
	PermSessionsWrite = "sessions:write"
)
