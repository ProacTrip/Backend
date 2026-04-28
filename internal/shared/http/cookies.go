package http

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

// =============================================================================
// Cookie Helper - Funciones para setear auth cookies según la documentación
// =============================================================================

const (
	// Cookie names según la doc
	accessCookieName  = "__Secure-access_token"
	refreshCookieName = "__Secure-refresh_token"

	// TTLs según la doc
	accessTTL  = 15 * time.Minute   // 900 segundos
	refreshTTL = 7 * 24 * time.Hour // 604800 segundos

	// Config de cookies para producción
	cookieDomain   = ".proactrip.com" // Cambiar en producción
	cookiePath     = "/"
	cookieSameSite = http.SameSiteLaxMode
)

// SetAuthCookiesFromTokens configura las cookies de autenticación
// según la documentación AUTH_API_V3.md:
// - HttpOnly: true (inaccesible vía JavaScript)
// - Secure: true (solo HTTPS)
// - SameSite: Lax (protección CSRF, permite OAuth callbacks)
// - Partitioned: true (CHIPS - permite en contextos terceros)
// - Domain: .proactrip.com (compartido entre subdominios)
func SetAuthCookiesFromTokens(c *echo.Context, accessToken, refreshToken string) error {
	// Access token cookie - producción multi-subdominio
	accessCookie := &http.Cookie{
		Name:     accessCookieName,
		Value:    accessToken,
		Path:     cookiePath,
		Domain:   cookieDomain,
		MaxAge:   int(accessTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: cookieSameSite,
		// Partitioned: true // Go http.Cookie no tiene este campo directamente
	}

	// Refresh token cookie
	refreshCookie := &http.Cookie{
		Name:     refreshCookieName,
		Value:    refreshToken,
		Path:     cookiePath,
		Domain:   cookieDomain,
		MaxAge:   int(refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: cookieSameSite,
	}

	// Production: usar solo Header().Add() con el string completo incluyendo Partitioned
	// NO usar SetCookie() porque enviaría duplicate Set-Cookie headers
	c.Response().Header().Add("Set-Cookie", accessCookie.String()+"; Partitioned")
	c.Response().Header().Add("Set-Cookie", refreshCookie.String()+"; Partitioned")

	return nil
}

// SetAuthCookiesDev es igual pero sin domain y sin prefijo __Secure- (para desarrollo local sin HTTPS).
// El prefijo __Secure- requiere Secure=true que no está disponible en localhost.
func SetAuthCookiesDev(c *echo.Context, accessToken, refreshToken string) error {
	accessCookie := &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     cookiePath,
		MaxAge:   int(accessTTL.Seconds()),
		HttpOnly: true,
		// Secure: false para desarrollo
		SameSite: cookieSameSite,
	}

	refreshCookie := &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     cookiePath,
		MaxAge:   int(refreshTTL.Seconds()),
		HttpOnly: true,
		SameSite: cookieSameSite,
	}

	c.SetCookie(accessCookie)
	c.SetCookie(refreshCookie)

	return nil
}

// ClearAuthCookies limpia las cookies de autenticación (logout)
// Envía Clear-Site-Data header + cookies con Max-Age=0 y Partitioned
func ClearAuthCookies(c *echo.Context) error {
	accessCookie := &http.Cookie{
		Name:     accessCookieName,
		Value:    "",
		Path:     cookiePath,
		Domain:   cookieDomain,
		MaxAge:   0,
		HttpOnly: true,
		Secure:   true,
		SameSite: cookieSameSite,
	}

	refreshCookie := &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     cookiePath,
		Domain:   cookieDomain,
		MaxAge:   0,
		HttpOnly: true,
		Secure:   true,
		SameSite: cookieSameSite,
	}

	c.Response().Header().Add("Set-Cookie", accessCookie.String()+"; Partitioned")
	c.Response().Header().Add("Set-Cookie", refreshCookie.String()+"; Partitioned")
	c.Response().Header().Set("Clear-Site-Data", `"cookies"`)

	return nil
}

// GetAccessTokenFromCookie obtiene el access token de la cookie
func GetAccessTokenFromCookie(c *echo.Context) string {
	cookie, err := c.Cookie(accessCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// GetRefreshTokenFromCookie obtiene el refresh token de la cookie
func GetRefreshTokenFromCookie(c *echo.Context) string {
	cookie, err := c.Cookie(refreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
