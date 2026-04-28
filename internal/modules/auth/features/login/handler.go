package login

import (
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler HTTP para login.
// Valida credenciales y setea cookies de sesión.

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

	var cmd Command
	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	if resp.AccessToken != "" && resp.RefreshToken != "" {
		if h.isProduction {
			httperr.SetAuthCookiesFromTokens(c, resp.AccessToken, resp.RefreshToken)
		} else {
			httperr.SetAuthCookiesDev(c, resp.AccessToken, resp.RefreshToken)
		}
	}

	return c.JSON(http.StatusOK, resp)
}
