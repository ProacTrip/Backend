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

	// Config de cookies
	cookiePath     = "/"
	cookieSameSite = http.SameSiteLaxMode
)

// SetAuthCookiesFromTokens configura las cookies de autenticación
// según la documentación AUTH_API_V3.md:
// - HttpOnly: true (inaccesible vía JavaScript)
// - Secure: true (solo HTTPS)
// - SameSite: Lax (protección CSRF, permite OAuth callbacks)
// NOTA: NO usamos Partitioned (CHIPS) porque:
// 1. Proactrip usa subdominios (api.proactrip.com, app.proactrip.com)
// 2. Partitioned es para iframes/cross-site embedding, no subdominios
// 3. Con Partitioned + Domain amplio, las cookies no se envían entre subdominios
// 4. SameSite=Lax + Domain=.proactrip.com es suficiente para compartir cookies
// - Domain: configurable desde COOKIE_DOMAIN (ej. .proactrip.com en prod, vacío en dev)
func SetAuthCookiesFromTokens(c *echo.Context, accessToken, refreshToken, cookieDomain string) error {
	// Access token cookie - producción multi-subdominio
	accessCookie := &http.Cookie{
		Name:     accessCookieName,
		Value:    accessToken,
		Path:     cookiePath,
		MaxAge:   int(accessTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: cookieSameSite,
	}
	if cookieDomain != "" {
		accessCookie.Domain = cookieDomain
	}

	// Refresh token cookie
	refreshCookie := &http.Cookie{
		Name:     refreshCookieName,
		Value:    refreshToken,
		Path:     cookiePath,
		MaxAge:   int(refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: cookieSameSite,
	}
	if cookieDomain != "" {
		refreshCookie.Domain = cookieDomain
	}

	// Production: usar Header().Add() con el string completo
	c.Response().Header().Add("Set-Cookie", accessCookie.String())
	c.Response().Header().Add("Set-Cookie", refreshCookie.String())

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
// Envía Clear-Site-Data header + cookies con Max-Age=0.
// En Go, http.Cookie{MaxAge: -1} genera "Max-Age=0" en el header HTTP.
// MaxAge: 0 omite el atributo (cookie de sesión, no se borra).
func ClearAuthCookies(c *echo.Context, cookieDomain string) error {
	accessCookie := &http.Cookie{
		Name:     accessCookieName,
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1, // genera Max-Age=0 → borra inmediatamente
		HttpOnly: true,
		Secure:   true,
		SameSite: cookieSameSite,
	}
	if cookieDomain != "" {
		accessCookie.Domain = cookieDomain
	}

	refreshCookie := &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1, // genera Max-Age=0 → borra inmediatamente
		HttpOnly: true,
		Secure:   true,
		SameSite: cookieSameSite,
	}
	if cookieDomain != "" {
		refreshCookie.Domain = cookieDomain
	}

	c.Response().Header().Add("Set-Cookie", accessCookie.String())
	c.Response().Header().Add("Set-Cookie", refreshCookie.String())
	c.Response().Header().Set("Clear-Site-Data", `"cookies"`)

	return nil
}

// ClearAuthCookiesDev is the dev-only variant clearing access_token/refresh_token
// via c.SetCookie() with MaxAge=-1 (generates Max-Age=0), HttpOnly, SameSite=Lax.
// No Secure, no Clear-Site-Data header. Matches SetAuthCookiesDev pattern.
func ClearAuthCookiesDev(c *echo.Context) error {
	accessCookie := &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1, // genera Max-Age=0 → borra inmediatamente
		HttpOnly: true,
		SameSite: cookieSameSite,
	}

	refreshCookie := &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1, // genera Max-Age=0 → borra inmediatamente
		HttpOnly: true,
		SameSite: cookieSameSite,
	}

	c.SetCookie(accessCookie)
	c.SetCookie(refreshCookie)

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
