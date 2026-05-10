package callback

import (
	"fmt"
	"net/http"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

// Handler HTTP para el callback OAuth.
// GET /v1/auth/oauth/:provider/callback?code=xxx&state=yyy
// Procesa el callback, setea cookies y redirige al frontend.

type Handler struct {
	usecase      *UseCase
	isProduction bool
	frontendURL  string
	cookieDomain string
}

func NewHandler(usecase *UseCase, isProduction bool, frontendURL string, cookieDomain string) *Handler {
	return &Handler{
		usecase:      usecase,
		isProduction: isProduction,
		frontendURL:  frontendURL,
		cookieDomain: cookieDomain,
	}
}

// Handle procesa el callback OAuth de Google.
// Extrae code y state de los query params, ejecuta el usecase,
// setea cookies de sesión y redirige al frontend con status=success.
// En caso de error, redirige con status=error&code=XXX.
func (h *Handler) Handle(c *echo.Context) error {
	// Check for provider error first (user denied, etc.)
	if errorParam, _ := echo.QueryParamOr[string](c, "error", ""); errorParam != "" {
		redirectURL := fmt.Sprintf("%s/auth/callback?status=error&code=OAUTH_ACCESS_DENIED", h.frontendURL)
		return c.Redirect(http.StatusFound, redirectURL)
	}

	// Extraer provider del path param
	provider, err := echo.PathParam[string](c, "provider")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid provider")
	}

	// Extraer code y state de los query params
	code, err := echo.QueryParam[string](c, "code")
	if err != nil || code == "" {
		return h.redirectError(c, "OAUTH_CODE_MISSING")
	}

	state, err := echo.QueryParam[string](c, "state")
	if err != nil || state == "" {
		return h.redirectError(c, "OAUTH_STATE_MISSING")
	}

	cmd := Command{
		ProviderCode: code,
		State:        state,
		Provider:     provider,
	}

	if err := cmd.Validate(); err != nil {
		return h.redirectError(c, h.errorCodeFrom(err))
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return h.redirectError(c, h.errorCodeFrom(err))
	}

	// Setear cookies de autenticación
	if resp.AccessToken != "" && resp.RefreshToken != "" {
		if h.isProduction {
			httperr.SetAuthCookiesFromTokens(c, resp.AccessToken, resp.RefreshToken, h.cookieDomain)
		} else {
			httperr.SetAuthCookiesDev(c, resp.AccessToken, resp.RefreshToken)
		}
	}

	// 302 Redirect al frontend
	redirectURL := fmt.Sprintf("%s/auth/callback?status=success", h.frontendURL)
	return c.Redirect(http.StatusFound, redirectURL)
}

// redirectError redirige al frontend con código de error.
func (h *Handler) redirectError(c *echo.Context, errorCode string) error {
	redirectURL := fmt.Sprintf("%s/auth/callback?status=error&code=%s", h.frontendURL, errorCode)
	return c.Redirect(http.StatusFound, redirectURL)
}

// errorCodeFrom extrae el código de error del domain error para el redirect.
func (h *Handler) errorCodeFrom(err error) string {
	errStr := err.Error()
	// Extraer el código antes de los dos puntos (formato: "CODE: mensaje")
	for i, ch := range errStr {
		if ch == ':' {
			return errStr[:i]
		}
	}
	// Si no tiene formato CODE: mensaje → usar el error genérico
	return "OAUTH_EXCHANGE_FAILED"
}
