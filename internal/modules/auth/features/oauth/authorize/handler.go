package authorize

import (
	"log/slog"
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler HTTP para la autorización OAuth.
// GET /v1/auth/oauth/:provider → retorna { auth_url } para que el frontend redirija.

type Handler struct {
	usecase *UseCase
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle procesa la solicitud de autorización OAuth.
// Extrae el provider del path param, ejecuta el usecase y retorna la auth_url.
func (h *Handler) Handle(c *echo.Context) error {
	// No cachear esta respuesta — cada llamada genera un state nuevo
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Content-Type", "application/json")

	provider, err := echo.PathParam[string](c, "provider")
	if err != nil || provider == "" {
		return httperr.MapError(c, echo.NewHTTPError(http.StatusBadRequest, "provider es requerido"))
	}

	cmd := Command{Provider: provider}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	// NO loguear auth_url — contiene state token sensible
	slog.InfoContext(c.Request().Context(), "oauth authorize", "provider", provider)

	return c.JSON(http.StatusOK, resp)
}
