// Manejo de cookies para usuarios anónimos.
// Genera y persiste ID único via cookie HttpOnly para rate limiting.
package ratelimit

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

const (
	AnonCookieName      = "__Secure-anon_token"
	AnonContextKey      = "anon_id"
	AnonCookieMaxAgeSec = 315360000 // 10 years
	AnonCookieDomain    = ".proactrip.com"
)

// AnonymousCookieMiddleware generates and persists a unique anonymous user ID
// via a long-lived HttpOnly cookie. The cookie uses __Secure- prefix and Domain
// in production, and plain names in development. The anon ID is stored in the
// Echo context under AnonContextKey for downstream rate-limit middleware.
func AnonymousCookieMiddleware(skipper func(c *echo.Context) bool, isProduction bool) echo.MiddlewareFunc {
	if skipper == nil {
		skipper = func(c *echo.Context) bool { return false }
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if skipper(c) {
				return next(c)
			}

			anonID := extractOrGenerateAnonID(c, isProduction)
			c.Set(AnonContextKey, anonID)
			return next(c)
		}
	}
}

func extractOrGenerateAnonID(c *echo.Context, isProduction bool) string {
	cookie, err := c.Cookie(AnonCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	anonID := uuid.Must(uuid.NewV7()).String()

	cookie = &http.Cookie{
		Name:     AnonCookieName,
		Value:    anonID,
		Path:     "/",
		MaxAge:   AnonCookieMaxAgeSec,
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
	}

	setCookieStr := func() string {
		if isProduction {
			return fmt.Sprintf("%s; Domain=%s",
				cookie.String(), AnonCookieDomain)
		}
		return cookie.String()
	}()

	c.Response().Header().Add("Set-Cookie", setCookieStr)

	return anonID
}

// AnonIDFromContext extracts the anonymous user ID from the Echo context.
// Returns empty string if none was set (caller skipped AnonymousCookieMiddleware).
func AnonIDFromContext(c *echo.Context) string {
	if v := c.Get(AnonContextKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
