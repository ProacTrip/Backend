// Middleware de autorización RequirePermission.
// Verifica que el usuario autenticado tenga un permiso específico.
// Sigue el mismo patrón que RequireAdmin.
//
// Modo observe (AUTHZ_ENFORCE_MODE=observe): loguea el deny pero nunca bloquea.
// Útil para rollout progresivo sin romper sesiones existentes.
package middleware

import (
	"errors"
	"log/slog"
	"os"
	"slices"

	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// PermissionClaims — interfaz que deben implementar los claims de autenticación
// =============================================================================

// PermissionClaims es la interfaz mínima para claims con permisos.
// Es satisfecha por shared/auth.AccessClaims vía GetPermissions() []string.
//
// Compile-time check: AccessClaims implementa esta interfaz.
type PermissionClaims interface {
	GetPermissions() []string
}

// =============================================================================
// ErrMissingPermission — error de autorización
// =============================================================================

// ErrMissingPermission se retorna cuando el usuario no tiene el permiso requerido.
// Mapeado a 403 Forbidden en el error mapper del módulo auth.
var ErrMissingPermission = errors.New("MISSING_PERMISSION: el usuario no tiene el permiso requerido")

// =============================================================================
// RequirePermission — middleware de autorización
// =============================================================================

// RequirePermission retorna un middleware de Echo que verifica que el usuario
// autenticado tenga el permiso especificado. Lee los claims del contexto
// (inyectados por el middleware de autenticación) y usa slices.Contains
// para verificar membresía.
//
// Uso:
//
//	e.GET("/v1/dashboard/users", handler, middleware.RequirePermission("users:read"))
//
// Modo observe:
//
//	Cuando AUTHZ_ENFORCE_MODE=observe, el middleware ejecuta la verificación completa
//	pero nunca retorna 403. En su lugar, loguea "authz would deny" y siempre llama
//	a next(c). Esto permite medir el impacto antes de activar la enforce real.
func RequirePermission(requiredPermission string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			claims, err := echo.ContextGet[PermissionClaims](c, "user_claims")
			if err != nil {
				// Claims no presentes o tipo incorrecto → error interno
				return httperr.MapError(c, serrors.ErrInternalError(
					"error interno de autorización: claims no disponibles",
					err,
				))
			}

			hasPermission := slices.Contains(claims.GetPermissions(), requiredPermission)

			// Modo observe: loguear pero nunca bloquear
			if isObserveMode() {
				if !hasPermission {
					slog.WarnContext(c.Request().Context(), "authz would deny",
						slog.String("permission", requiredPermission),
						slog.String("path", c.Request().URL.Path),
					)
				}
				return next(c)
			}

			// Modo enforce: bloquear si falta el permiso
			if !hasPermission {
				return httperr.MapError(c, serrors.ErrForbidden("permiso insuficiente", ErrMissingPermission))
			}

			return next(c)
		}
	}
}

// =============================================================================
// isObserveMode — verifica el modo de rollout de autorización
// =============================================================================

// isObserveMode retorna true cuando AUTHZ_ENFORCE_MODE está seteado a "observe".
// En este modo, el middleware ejecuta la verificación pero nunca bloquea requests.
// Valores: "observe" (solo log), "enforce" o vacío (enforce real).
func isObserveMode() bool {
	return os.Getenv("AUTHZ_ENFORCE_MODE") == "observe"
}

// =============================================================================
// init — registro del error mapper para ErrMissingPermission
// =============================================================================

func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		if errors.Is(err, ErrMissingPermission) {
			return serrors.ErrForbidden("permiso insuficiente", err)
		}
		return nil
	})
}
