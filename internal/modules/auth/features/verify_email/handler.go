package verify_email

import (
	"net/http"
	"strings"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler HTTP para verificación de email.
// Recibe token de verificación, extrae idioma del header Accept-Language,
// y setea cookies de sesión tras verificar exitosamente.

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

	// Extraer código de idioma del header Accept-Language.
	// Ejemplo: "es-MX,en;q=0.9" → "es"
	language := ""
	acceptLang := c.Request().Header.Get("Accept-Language")
	if acceptLang != "" {
		// Tomar el primer locale: "es-MX,en;q=0.9" → "es-MX"
		parts := strings.SplitN(acceptLang, ",", 2)
		primary := strings.TrimSpace(parts[0])
		// Extraer el subtag primario: "es-MX" → "es"
		if idx := strings.Index(primary, "-"); idx != -1 {
			language = primary[:idx]
		} else {
			language = primary
		}
	}

	var cmd Command
	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	if err := cmd.Validate(); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd, language)
	if err != nil {
		return httperr.MapError(c, err)
	}

	if resp.AccessToken != "" && resp.RefreshToken != "" {
		if h.isProduction {
			httperr.SetAuthCookiesFromTokens(c, resp.AccessToken, resp.RefreshToken, h.cookieDomain)
		} else {
			httperr.SetAuthCookiesDev(c, resp.AccessToken, resp.RefreshToken)
		}
	}

	return c.JSON(http.StatusOK, resp)
}
