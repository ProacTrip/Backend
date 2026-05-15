// Handler HTTP para detalle de usuario del dashboard.
// Expuesto en GET /v1/dashboard/users/:id.
package user_detail

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// =============================================================================
// Handler — endpoint HTTP de user detail
// =============================================================================

// Handler processes user detail HTTP requests.
type Handler struct {
	usecase *UseCase
}

// NewHandler creates a new user detail handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle processes the user detail request.
// Route: GET /v1/dashboard/users/:id
// Path param :id is extracted via echo.PathParam[uuid.UUID].
func (h *Handler) Handle(c *echo.Context) error {
	userID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	cmd := Command{UserID: userID}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
