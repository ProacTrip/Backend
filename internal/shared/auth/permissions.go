// Constantes de permisos para el sistema de autorización RBAC.
// Los 5 permisos base del dashboard admin.
// Usados por RequirePermission middleware y dashboard endpoints.
package auth

// Permisos del módulo dashboard — coinciden con los seeds en permissions table.
// Formato: "resource:action" (ej. "users:read").
const (
	// Lectura y escritura de usuarios del dashboard.
	PermUsersRead  = "users:read"
	PermUsersWrite = "users:write"

	// Escritura de feature limits.
	PermFeatureLimitsWrite = "feature_limits:write"


)
