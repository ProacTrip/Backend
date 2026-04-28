package get_context

import (
	"net/http"

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
	lang := c.QueryParam("lang")
	if lang == "" {
		lang = "en"
	}

	result, err := h.useCase.Execute(c.Request().Context(), ip, lang)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}
