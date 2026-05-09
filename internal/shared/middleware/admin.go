package middleware

import (
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/labstack/echo/v5"
)

func RequireAdmin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			claims, err := echo.ContextGet[token.RoleClaims](c, "user_claims")
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
