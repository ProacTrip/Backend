package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

const (
	accessCookieNameProd  = "__Secure-access_token"
	refreshCookieNameProd = "__Secure-refresh_token"
	accessCookieNameDev   = "access_token"
	refreshCookieNameDev  = "refresh_token"

	cookiePath     = "/"
	cookieSameSite = http.SameSiteLaxMode
	accessTTL      = 15 * time.Minute
	refreshTTL     = 7 * 24 * time.Hour
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
				m.setAuthCookies(c, newAccess, newRefresh)
				c.Set("user_claims", claims)
				return next(c)
			}
			if errors.Is(err, domain.ErrTokenRevoked) || errors.Is(err, domain.ErrTokenExpired) {
				m.clearAuthCookies(c)
				return c.JSON(http.StatusUnauthorized, serrors.ErrUnauthorized(
					"Sesión expirada. Inicia sesión nuevamente.", err,
				).WithInstance(c.Request().URL.Path))
			}
		}

		m.clearAuthCookies(c)
		return c.JSON(http.StatusUnauthorized, serrors.ErrUnauthorized(
			"Autenticación requerida", nil,
		).WithInstance(c.Request().URL.Path))
	}
}

func (m *AuthMiddleware) cookieNames() (string, string) {
	if m.config.IsProduction {
		return accessCookieNameProd, refreshCookieNameProd
	}
	return accessCookieNameDev, refreshCookieNameDev
}

func (m *AuthMiddleware) setAuthCookies(c *echo.Context, accessToken, refreshToken string) {
	accessName, refreshName := m.cookieNames()

	if m.config.IsProduction {
		accessCookie := &http.Cookie{
			Name:     accessName,
			Value:    accessToken,
			Path:     cookiePath,
			Domain:   m.config.CookieDomain,
			MaxAge:   int(accessTTL.Seconds()),
			HttpOnly: true,
			Secure:   true,
			SameSite: cookieSameSite,
		}
		refreshCookie := &http.Cookie{
			Name:     refreshName,
			Value:    refreshToken,
			Path:     cookiePath,
			Domain:   m.config.CookieDomain,
			MaxAge:   int(refreshTTL.Seconds()),
			HttpOnly: true,
			Secure:   true,
			SameSite: cookieSameSite,
		}
		c.Response().Header().Add("Set-Cookie", accessCookie.String()+"; Partitioned")
		c.Response().Header().Add("Set-Cookie", refreshCookie.String()+"; Partitioned")
	} else {
		accessCookie := &http.Cookie{
			Name:     accessName,
			Value:    accessToken,
			Path:     cookiePath,
			MaxAge:   int(accessTTL.Seconds()),
			HttpOnly: true,
			SameSite: cookieSameSite,
		}
		refreshCookie := &http.Cookie{
			Name:     refreshName,
			Value:    refreshToken,
			Path:     cookiePath,
			MaxAge:   int(refreshTTL.Seconds()),
			HttpOnly: true,
			SameSite: cookieSameSite,
		}
		c.SetCookie(accessCookie)
		c.SetCookie(refreshCookie)
	}
}

func (m *AuthMiddleware) clearAuthCookies(c *echo.Context) {
	accessName, refreshName := m.cookieNames()

	if m.config.IsProduction {
		accessCookie := &http.Cookie{
			Name:     accessName,
			Value:    "",
			Path:     cookiePath,
			Domain:   m.config.CookieDomain,
			MaxAge:   0,
			HttpOnly: true,
			Secure:   true,
			SameSite: cookieSameSite,
		}
		refreshCookie := &http.Cookie{
			Name:     refreshName,
			Value:    "",
			Path:     cookiePath,
			Domain:   m.config.CookieDomain,
			MaxAge:   0,
			HttpOnly: true,
			Secure:   true,
			SameSite: cookieSameSite,
		}
		c.Response().Header().Add("Set-Cookie", accessCookie.String()+"; Partitioned")
		c.Response().Header().Add("Set-Cookie", refreshCookie.String()+"; Partitioned")
	} else {
		accessCookie := &http.Cookie{
			Name:     accessName,
			Value:    "",
			Path:     cookiePath,
			MaxAge:   0,
			HttpOnly: true,
			SameSite: cookieSameSite,
		}
		refreshCookie := &http.Cookie{
			Name:     refreshName,
			Value:    "",
			Path:     cookiePath,
			MaxAge:   0,
			HttpOnly: true,
			SameSite: cookieSameSite,
		}
		c.SetCookie(accessCookie)
		c.SetCookie(refreshCookie)
	}
}
