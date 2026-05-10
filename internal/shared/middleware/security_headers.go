package middleware

// Middleware que agrega headers de seguridad HTTP:
// CSP, X-Frame-Options, HSTS, etc.
import "github.com/labstack/echo/v5"

// SecurityHeaders is an Echo middleware that sets standard security headers:
// CSP (default-src 'self'), X-Content-Type-Options (nosniff),
// X-Frame-Options (DENY), Referrer-Policy (strict-origin-when-cross-origin),
// and Strict-Transport-Security (max-age=31536000; includeSubDomains).
func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Content-Security-Policy", "default-src 'self'")
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("X-Frame-Options", "DENY")
			c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			c.Response().Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			return next(c)
		}
	}
}
