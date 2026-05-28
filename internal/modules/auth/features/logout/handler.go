package logout

import (
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

const (
	accessCookieNameProd = "__Secure-access_token"
	accessCookieNameDev  = "access_token"
)

// Handler HTTP para logout.
// Extrae token de la cookie, invalida sesión y limpia cookies.

type Handler struct {
	usecase      *UseCase
	isProduction bool
	cookieDomain string
	frontendURL  string
}

func NewHandler(usecase *UseCase, isProduction bool, cookieDomain, frontendURL string) *Handler {
	return &Handler{
		usecase:      usecase,
		isProduction: isProduction,
		cookieDomain: cookieDomain,
		frontendURL:  frontendURL,
	}
}

func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	// Always clear cookies first — even if the token is invalid or missing.
	if h.isProduction {
		httperr.ClearAuthCookies(c, h.cookieDomain)
	} else {
		httperr.ClearAuthCookiesDev(c)
	}

	tokenStr := h.extractToken(c)

	// Best-effort: if we have a valid token, invalidate the session.
	if tokenStr != "" {
		cmd := Command{Token: tokenStr}
		if err := cmd.Validate(); err == nil {
			h.usecase.Execute(c.Request().Context(), cmd)
		}
	}

	// Redirect to frontend home — browser processes Set-Cookie + follows redirect
	return c.Redirect(http.StatusFound, h.frontendURL+"/")
}

func (h *Handler) extractToken(c *echo.Context) string {
	cookie, err := c.Cookie(accessCookieNameProd)
	if err != nil {
		cookie, err = c.Cookie(accessCookieNameDev)
	}
	if err != nil {
		return ""
	}
	return cookie.Value
}
