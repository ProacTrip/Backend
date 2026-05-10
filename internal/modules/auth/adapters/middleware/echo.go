package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	sharedhttp "github.com/ProacTrip/Backend/internal/shared/http"
)

const (
	accessCookieNameProd  = "__Secure-access_token"
	refreshCookieNameProd = "__Secure-refresh_token"
	accessCookieNameDev   = "access_token"
	refreshCookieNameDev  = "refresh_token"
)

type TokenService interface {
	ValidateAccessToken(ctx context.Context, tokenString string) (*token.AccessClaims, error)
	ValidateRefreshToken(ctx context.Context, tokenString string) (*token.RefreshClaims, error)
	ValidateAndRotateRefresh(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error)
}

type AuthConfig struct {
	IsProduction bool
	TokenSvc     TokenService
	UserRepo     domain.UserRepository
	CookieDomain string
}

type AuthMiddleware struct {
	config AuthConfig
}

func NewAuthMiddleware(cfg AuthConfig) *AuthMiddleware {
	return &AuthMiddleware{config: cfg}
}

func (m *AuthMiddleware) Handle(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		accessName, refreshName := m.cookieNames()

		accessCookie, _ := c.Cookie(accessName)
		refreshCookie, _ := c.Cookie(refreshName)

		if accessCookie == nil && refreshCookie == nil {
			return next(c)
		}

		if accessCookie != nil && accessCookie.Value != "" {
			claims, err := m.config.TokenSvc.ValidateAccessToken(c.Request().Context(), accessCookie.Value)
			if err == nil {
				c.Set("user_claims", claims)
				return next(c)
			}
		}

		if refreshCookie != nil && refreshCookie.Value != "" {
			claims, newAccess, newRefresh, err := m.config.TokenSvc.ValidateAndRotateRefresh(c.Request().Context(), refreshCookie.Value)
			if err == nil {
			if m.config.IsProduction {
				sharedhttp.SetAuthCookiesFromTokens(c, newAccess, newRefresh, m.config.CookieDomain)
			} else {
				sharedhttp.SetAuthCookiesDev(c, newAccess, newRefresh)
			}
			c.Set("user_claims", claims)
			return next(c)
		}
		if errors.Is(err, domain.ErrTokenRevoked) || errors.Is(err, domain.ErrTokenExpired) {
			if m.config.IsProduction {
				sharedhttp.ClearAuthCookies(c, m.config.CookieDomain)
			} else {
				sharedhttp.ClearAuthCookiesDev(c)
			}
				return c.JSON(http.StatusUnauthorized, serrors.ErrUnauthorized(
					"Sesión expirada. Inicia sesión nuevamente.", err,
				).WithInstance(c.Request().URL.Path))
			}
		}

	if m.config.IsProduction {
		sharedhttp.ClearAuthCookies(c, m.config.CookieDomain)
	} else {
		sharedhttp.ClearAuthCookiesDev(c)
	}
	return c.JSON(http.StatusUnauthorized, serrors.ErrUnauthorized(
		"Autenticación requerida", nil,
	).WithInstance(c.Request().URL.Path))
	}
}

// Optional returns a middleware that extracts user claims when valid credentials
// are present, but NEVER rejects the request. Invalid/expired tokens are silently
// ignored — the request proceeds as anonymous with no user_claims in context.
//
// Use this on public endpoints that behave differently for authenticated users
// (e.g., higher rate limits, profile preferences, conversation persistence)
// but must remain accessible to anonymous users.
func (m *AuthMiddleware) Optional() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			accessName, refreshName := m.cookieNames()

			accessCookie, _ := c.Cookie(accessName)
			refreshCookie, _ := c.Cookie(refreshName)

			// No cookies → anonymous, pass through
			if accessCookie == nil && refreshCookie == nil {
				return next(c)
			}

			// Try access token first
			if accessCookie != nil && accessCookie.Value != "" {
				claims, err := m.config.TokenSvc.ValidateAccessToken(c.Request().Context(), accessCookie.Value)
				if err == nil {
					c.Set("user_claims", claims)
					return next(c)
				}
			}

			// Try refresh token rotation
			if refreshCookie != nil && refreshCookie.Value != "" {
				claims, newAccess, newRefresh, err := m.config.TokenSvc.ValidateAndRotateRefresh(c.Request().Context(), refreshCookie.Value)
				if err == nil {
			if m.config.IsProduction {
				sharedhttp.SetAuthCookiesFromTokens(c, newAccess, newRefresh, m.config.CookieDomain)
			} else {
				sharedhttp.SetAuthCookiesDev(c, newAccess, newRefresh)
			}
			c.Set("user_claims", claims)
			return next(c)
		}
	}

	// Token validation failed — proceed as anonymous.
			// Do NOT clear cookies (they belong to other routes).
			// Do NOT return 401 (this is a public endpoint).
			return next(c)
		}
	}
}

func (m *AuthMiddleware) cookieNames() (string, string) {
	if m.config.IsProduction {
		return accessCookieNameProd, refreshCookieNameProd
	}
	return accessCookieNameDev, refreshCookieNameDev
}
