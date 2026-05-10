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
	SessionID uuid.UUID
	JTI       uuid.UUID
}

// GetUserID retorna el user ID como UUID. Satisface RoleClaims interface.
func (c AccessClaims) GetUserID() uuid.UUID { return c.UserID }

// GetRole retorna el nombre del rol. Satisface RoleClaims interface.
func (c AccessClaims) GetRole() string { return c.Role }

// =============================================================================
// Helper de extracción del contexto
// =============================================================================

// GetAccessClaims extrae los claims del contexto de Echo.
// Retorna error si los claims no están presentes o el tipo no coincide.
// Uso: claims, err := sharedauth.GetAccessClaims(c)
func GetAccessClaims(c *echo.Context) (*AccessClaims, error) {
	return echo.ContextGet[*AccessClaims](c, "user_claims")
}
