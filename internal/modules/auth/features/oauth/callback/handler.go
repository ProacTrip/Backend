package callback

import (
	"errors"
	"fmt"
	"log/slog"
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
		slog.ErrorContext(c.Request().Context(), "oauth callback error",
			slog.String("error", err.Error()),
			slog.String("code", h.errorCodeFrom(err)),
		)
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

// errorCodeFrom extrae el código de dominio del error unwrapeando la cadena.
// Busca el código (texto antes de ":") en cada error de la cadena.
// Retorna el primer código encontrado que coincida con el patrón "CODIGO: mensaje".
// Si no encuentra ningún código válido, retorna el fallback OAUTH_EXCHANGE_FAILED.
func (h *Handler) errorCodeFrom(err error) string {
	current := err
	for current != nil {
		errStr := current.Error()
		for i, ch := range errStr {
			if ch == ':' {
				code := errStr[:i]
				// Validar que sea un código en mayúsculas con guiones bajos (formato de dominio)
				if len(code) >= 5 && isDomainCode(code) {
					return code
				}
				break
			}
		}
		current = errors.Unwrap(current)
	}
	return "OAUTH_EXCHANGE_FAILED"
}

// isDomainCode verifica si un string tiene formato de código de dominio
// (mayúsculas, guiones bajos, sin espacios).
func isDomainCode(s string) bool {
	for _, ch := range s {
		if ch >= 'A' && ch <= 'Z' {
			continue
		}
		if ch == '_' {
			continue
		}
		return false
	}
	return true
}
