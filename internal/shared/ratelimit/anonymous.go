// Manejo de cookies para usuarios anónimos.
// Genera y persiste ID único via cookie HttpOnly para rate limiting.
package ratelimit

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

const (
	AnonCookieName      = "anon_id"
	AnonContextKey      = "anon_id"
	AnonCookieMaxAgeSec = 86400 * 365 // 1 year
)

func AnonymousCookieMiddleware(skipper func(c *echo.Context) bool) echo.MiddlewareFunc {
	if skipper == nil {
		skipper = func(c *echo.Context) bool { return false }
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if skipper(c) {
				return next(c)
			}

			anonID := extractOrGenerateAnonID(c)
			c.Set(AnonContextKey, anonID)
			return next(c)
		}
	}
}

func extractOrGenerateAnonID(c *echo.Context) string {
	cookie, err := c.Cookie(AnonCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	anonID := uuid.Must(uuid.NewV7()).String()

	cookie = new(http.Cookie)
	cookie.Name = AnonCookieName
	cookie.Value = anonID
	cookie.Path = "/"
	cookie.MaxAge = AnonCookieMaxAgeSec
	cookie.HttpOnly = true
	cookie.Secure = true
	cookie.SameSite = http.SameSiteLaxMode
	c.SetCookie(cookie)

	return anonID
}

func AnonIDFromContext(c *echo.Context) string {
	if v := c.Get(AnonContextKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
