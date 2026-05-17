package get_environment

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
)

type Handler struct {
	useCase *UseCase
}

func NewHandler(useCase *UseCase) *Handler {
	return &Handler{useCase: useCase}
}

func (h *Handler) Handle(c *echo.Context) error {
	slog.DebugContext(c.Request().Context(), "get_environment handler: request received")

	// Resolver IP: X-Real-IP tiene precedencia sobre c.RealIP()
	ip := c.Request().Header.Get("X-Real-IP")
	if ip == "" {
		ip = c.RealIP()
	}
	slog.DebugContext(c.Request().Context(), "get_environment handler: resolved IP", "ip", ip)

	// Validar IP: rechazar IPs privadas, loopback y malformadas (solo en producción)
	if os.Getenv("SERVER_ENV") != "dev" && domain.IsPrivateOrLocalIP(ip) {
		slog.WarnContext(c.Request().Context(), "get_environment handler: IP inválida o privada", "ip", ip)
		return httperr.MapError(c, domain.ErrInvalidIP)
	}

	lang := extractLanguage(c)
	slog.DebugContext(c.Request().Context(), "get_environment handler: extracted language", "lang", lang)

	slog.DebugContext(c.Request().Context(), "get_environment handler: calling useCase.Execute", "ip", ip, "lang", lang)
	result, err := h.useCase.Execute(c.Request().Context(), ip, lang)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "get_environment handler: useCase.Execute failed", "error", err)
		return httperr.MapError(c, err)
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=600")
	slog.DebugContext(c.Request().Context(), "get_environment handler: returning 200 OK")
	return c.JSON(http.StatusOK, result)
}

func extractLanguage(c *echo.Context) string {
	header := c.Request().Header.Get("Accept-Language")
	if header == "" {
		return "en"
	}

	// Extraer el primer idioma (antes de coma)
	first, _, found := strings.Cut(header, ",")
	if found {
		first = strings.TrimSpace(first)
	} else {
		first = strings.TrimSpace(header)
	}

	// Remover parámetro de calidad (;q=...)
	if idx := strings.IndexByte(first, ';'); idx != -1 {
		first = strings.TrimSpace(first[:idx])
	}

	// Extraer el código de idioma primario antes del guion regional
	if idx := strings.IndexByte(first, '-'); idx != -1 {
		first = first[:idx]
	}

	if len(first) >= 2 {
		return strings.ToLower(first[:2])
	}
	return "en"
}
