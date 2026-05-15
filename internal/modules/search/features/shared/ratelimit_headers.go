// Helper para setear headers de rate limit de provider en respuestas HTTP.
package shared

import (
	"strconv"

	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/labstack/echo/v5"
)

// defaultRetryAfterSeconds is the fallback Retry-After value (in seconds)
// used when the rate limiter is unavailable or its status cannot be queried.
// 60 seconds is a conservative default that avoids hammering providers
// while not blocking clients for longer than necessary.
const defaultRetryAfterSeconds = 60

// SetRateLimitHeaders setea RateLimit-Limit, RateLimit-Remaining y RateLimit-Reset
// desde un RateLimitResult en el header de la respuesta.
// Usa asignación directa (no h.Set) para evitar que net/http canonicalice
// "RateLimit-" a "Ratelimit-" (comportamiento de RFC 2616).
func SetRateLimitHeaders(c *echo.Context, result ratelimit.RateLimitResult) {
	h := c.Response().Header()
	h["RateLimit-Limit"] = []string{strconv.FormatInt(result.Limit, 10)}
	h["RateLimit-Remaining"] = []string{strconv.FormatInt(result.Remaining, 10)}
	h["RateLimit-Reset"] = []string{strconv.FormatInt(int64(result.ResetTTL.Seconds()), 10)}
}

// SetRateLimitExceededHeaders setea Retry-After y RateLimit-* headers
// en el path de error 429 (provider rate limit exceeded).
// Debe llamarse ANTES de httperr.MapError para que los headers estén presentes
// en la respuesta de error.
func SetRateLimitExceededHeaders(c *echo.Context, rl *ratelimit.RateLimiter, provider string) {
	if rl == nil {
		c.Response().Header()["Retry-After"] = []string{strconv.Itoa(defaultRetryAfterSeconds)}
		return
	}
	result, err := rl.ProviderStatus(c.Request().Context(), provider)
	if err != nil {
		// Fallback si no se puede consultar el estado
		c.Response().Header()["Retry-After"] = []string{strconv.Itoa(defaultRetryAfterSeconds)}
		return
	}
	// Retry-After dinámico desde el TTL real del rate limiter
	c.Response().Header()["Retry-After"] = []string{strconv.FormatInt(int64(result.ResetTTL.Seconds()), 10)}
	SetRateLimitHeaders(c, result)
}
