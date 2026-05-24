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
}

func NewHandler(usecase *UseCase, isProduction bool, cookieDomain string) *Handler {
	return &Handler{
		usecase:      usecase,
		isProduction: isProduction,
		cookieDomain: cookieDomain,
	}
}

func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	tokenStr := h.extractToken(c)

	cmd := Command{Token: tokenStr}
	if err := cmd.Validate(); err != nil {
		return httperr.MapError(c, err)
	}

	_, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	if h.isProduction {
		httperr.ClearAuthCookies(c, h.cookieDomain)
	} else {
		httperr.ClearAuthCookiesDev(c)
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Sesión cerrada exitosamente."})
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
