package get_environment

import (
	"log/slog"
	"net/http"
	"strings"

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
	slog.Debug("get_environment handler: request received")

	if h.useCase == nil {
		slog.Error("get_environment handler: FATAL — useCase is nil")
		return httperr.MapError(c, echo.NewHTTPError(http.StatusInternalServerError, "environment service unavailable"))
	}

	ip := c.RealIP()
	slog.Debug("get_environment handler: extracted IP", "ip", ip)

	lang := extractLanguage(c)
	slog.Debug("get_environment handler: extracted language", "lang", lang)

	slog.Debug("get_environment handler: calling useCase.Execute", "ip", ip, "lang", lang)
	result, err := h.useCase.Execute(c.Request().Context(), ip, lang)
	if err != nil {
		slog.Error("get_environment handler: useCase.Execute failed", "error", err)
		return httperr.MapError(c, err)
	}

	slog.Debug("get_environment handler: returning 200 OK")
	return c.JSON(http.StatusOK, result)
}

func extractLanguage(c *echo.Context) string {
	header := c.Request().Header.Get("Accept-Language")
	if header == "" {
		return "en"
	}
	parts := strings.SplitN(header, ",", 2)
	first := strings.TrimSpace(parts[0])

	if idx := strings.IndexByte(first, ';'); idx != -1 {
		first = strings.TrimSpace(first[:idx])
	}

	if idx := strings.IndexByte(first, '-'); idx != -1 {
		first = first[:idx]
	}

	if len(first) >= 2 {
		return strings.ToLower(first[:2])
	}
	return "en"
}
