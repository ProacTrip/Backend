// Middlewares de Echo para aplicar rate limiting en requests.
// Incluye headers estándar RateLimit-*.
package ratelimit

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
)

// IdentifierFunc extrae el identificador (userID, IP) del request para rate limiting
type IdentifierFunc func(c *echo.Context) (string, bool)

// =============================================================================
// Middlewares de rate limiting por tipo de cliente
// =============================================================================

func GlobalRateLimitMiddleware(rl *RateLimiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ip := c.RealIP()
			result, err := rl.GlobalAllow(c.Request().Context(), ip)
			if err != nil {
				return fmt.Errorf("global rate limit: %w", err)
			}

			setRateLimitHeaders(c, result)
			if !result.Allowed {
				return rateLimitExceeded(c, result)
			}

			return next(c)
		}
	}
}

func AuthenticatedRateLimitMiddleware(rl *RateLimiter, extractor IdentifierFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			userID, ok := extractor(c)
			if !ok {
				return next(c)
			}

			result, err := rl.AuthenticatedAllow(c.Request().Context(), userID)
			if err != nil {
				return fmt.Errorf("auth rate limit: %w", err)
			}

			setRateLimitHeaders(c, result)
			if !result.Allowed {
				return rateLimitExceeded(c, result)
			}

			return next(c)
		}
	}
}

func AnonymousRateLimitMiddleware(rl *RateLimiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			anonID := AnonIDFromContext(c)
			if anonID == "" {
				return next(c)
			}

			result, err := rl.AnonymousAllow(c.Request().Context(), anonID)
			if err != nil {
				return fmt.Errorf("anon rate limit: %w", err)
			}

			setRateLimitHeaders(c, result)
			if !result.Allowed {
				return rateLimitExceeded(c, result)
			}

			return next(c)
		}
	}
}

func ProviderRateLimitMiddleware(rl *RateLimiter, provider string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			result, err := rl.ProviderAllow(c.Request().Context(), provider)
			if err != nil {
				return fmt.Errorf("provider rate limit: %w", err)
			}

			setRateLimitHeaders(c, result)
			if !result.Allowed {
				return rateLimitExceeded(c, result)
			}

			return next(c)
		}
	}
}

func setRateLimitHeaders(c *echo.Context, result RateLimitResult) {
	h := c.Response().Header()
	h.Set("RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
	h.Set("RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
	h.Set("RateLimit-Reset", strconv.FormatInt(int64(result.ResetTTL.Seconds()), 10))
}

func rateLimitExceeded(c *echo.Context, result RateLimitResult) *echo.HTTPError {
	retryAfter := int(result.ResetTTL.Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}

	c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))

	return echo.NewHTTPError(http.StatusTooManyRequests,
		fmt.Sprintf("rate limit exceeded: %d/%d, retry after %ds",
			result.Current, result.Limit, retryAfter),
	)
}
