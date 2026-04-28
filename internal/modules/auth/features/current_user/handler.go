package current_user

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

const (
	accessCookieNameProd = "__Secure-access_token"
	accessCookieNameDev  = "access_token"
)

// Handler HTTP para obtener el usuario actual.
// Extrae token de la cookie y retorna datos del usuario autenticado.

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

	resp, _ := h.usecase.Execute(c.Request().Context(), tokenStr)

	if resp == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"user": nil})
	}

	return c.JSON(http.StatusOK, resp)
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
