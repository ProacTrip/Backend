package middleware

import (
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// RequireClientRole is an Echo middleware that blocks requests unless the authenticated
// user has the "client" role. It reads claims from the context via echo.ContextGet[RoleClaims].
//
// Use on user module endpoints to ensure only end-users (not admins) can access
// profile, preferences, medical data, avatars, and documents.
func RequireClientRole() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			claims, err := echo.ContextGet[RoleClaims](c, "user_claims")
			if err != nil {
				return httperr.MapError(c, serrors.ErrUnauthorized("authentication required", err))
			}
			if claims.GetRole() != "client" {
				return httperr.MapError(c, serrors.ErrForbidden("client role required", nil))
			}
			return next(c)
		}
	}
}
