package verify_email

import (
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler HTTP para verificación de email.
// Recibe token de verificación y setea cookies de sesión al confirmar.

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

	if cmd.Token == "" {
		return httperr.MapError(c, echo.NewHTTPError(http.StatusBadRequest, "token is required"))
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
