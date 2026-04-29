package get_context

import (
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
	ip := c.RealIP()
	lang := extractLanguage(c)

	result, err := h.useCase.Execute(c.Request().Context(), ip, lang)
	if err != nil {
		return httperr.MapError(c, err)
	}

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
