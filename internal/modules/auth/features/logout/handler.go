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
}

func NewHandler(usecase *UseCase, isProduction bool) *Handler {
	return &Handler{
		usecase:      usecase,
		isProduction: isProduction,
	}
}

func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	tokenStr := h.extractToken(c)

	_, err := h.usecase.Execute(c.Request().Context(), Command{Token: tokenStr})
	if err != nil {
		return httperr.MapError(c, err)
	}

	httperr.ClearAuthCookies(c)

	return c.JSON(http.StatusOK, map[string]string{"message": "Logged out successfully."})
}

func (h *Handler) HandleAll(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	tokenStr := h.extractToken(c)

	_, err := h.usecase.Execute(c.Request().Context(), Command{Token: tokenStr, LogoutAll: true})
	if err != nil {
		return httperr.MapError(c, err)
	}

	httperr.ClearAuthCookies(c)

	return c.JSON(http.StatusOK, map[string]string{"message": "All sessions have been revoked."})
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
