package resend_verification

import (
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler HTTP para reenvío de verificación de email.
// Siempre retorna 200 OK para prevenir enumeración de usuarios.
// No setea cookies, no retorna datos de usuario.
// Es un endpoint público (sin auth middleware).

type Handler struct {
	usecase *UseCase
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle procesa las requests POST /v1/auth/resend-verification.
// Según spec: body {email}, siempre 200 OK con mensaje genérico.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	var cmd Command
	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	if err := cmd.Validate(); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
