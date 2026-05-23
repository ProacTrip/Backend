// Tipos compartidos de claims de autenticación.
// Extractos de token PASETO — usados por middleware y handlers.
package auth

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// AccessClaims — Claims del token de acceso
// =============================================================================

// AccessClaims contiene los claims extraídos de un access token PASETO válido.
// Es inyectado por el middleware de autenticación en el contexto de Echo
// bajo la key "user_claims".
type AccessClaims struct {
	UserID    uuid.UUID
	Email     string
	RoleID    uuid.UUID
	Role      string
	JTI       uuid.UUID

	// Permissions es la lista de permisos efectivos resueltos para el usuario.
	// Se popula desde el cache de sesión o el PermissionResolver en cache miss.
	// Vacío hasta que el middleware de sesión los resuelva (Batch 3).
	Permissions []string

	// TokenVersion es la versión del token del usuario al momento de emisión.
	// Se compara contra el cache de sesión para detectar tokens stale.
	TokenVersion int
}

// GetUserID retorna el user ID como UUID. Satisface RoleClaims interface.
func (c AccessClaims) GetUserID() uuid.UUID { return c.UserID }

// GetRole retorna el nombre del rol. Satisface RoleClaims interface.
func (c AccessClaims) GetRole() string { return c.Role }

// GetPermissions retorna los permisos efectivos del usuario.
// Satisface PermissionClaims interface (shared/middleware/permission.go).
func (c AccessClaims) GetPermissions() []string { return c.Permissions }

// GetTokenVersion retorna la versión del token al momento de emisión.
// Usado por el middleware de sesión para detectar tokens stale.
func (c AccessClaims) GetTokenVersion() int { return c.TokenVersion }

// =============================================================================
// Helper de extracción del contexto
// =============================================================================

// GetAccessClaims extrae los claims del contexto de Echo.
// Retorna error si los claims no están presentes o el tipo no coincide.
// Uso: claims, err := sharedauth.GetAccessClaims(c)
func GetAccessClaims(c *echo.Context) (*AccessClaims, error) {
	return echo.ContextGet[*AccessClaims](c, "user_claims")
}
