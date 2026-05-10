package middleware

import (
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// RoleClaims is the minimal interface for claims with role information.
// Satisfied by auth token claims (both AccessClaims and RefreshClaims).
//
// Compile-time check: auth/token.AccessClaims and auth/token.RefreshClaims
// satisfy this interface via their GetRole() string methods.
type RoleClaims interface {
	GetRole() string
}

// RequireAdmin is an Echo middleware that blocks requests unless the authenticated
// user has the "admin" role. It reads claims from the context via echo.ContextGet[RoleClaims].
func RequireAdmin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			claims, err := echo.ContextGet[RoleClaims](c, "user_claims")
			if err != nil {
				return httperr.MapError(c, serrors.ErrUnauthorized("authentication required", err))
			}
			if claims.GetRole() != "admin" {
				return httperr.MapError(c, serrors.ErrForbidden("admin access required", nil))
			}
			return next(c)
		}
	}
}
