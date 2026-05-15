// Handler HTTP para listado de usuarios del dashboard.
// Expuesto en GET /v1/dashboard/users.
package list_users

import (
	"net/http"

	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// =============================================================================
// Handler — endpoint HTTP de list users
// =============================================================================

// Handler processes user listing HTTP requests.
type Handler struct {
	usecase *UseCase
}

// NewHandler creates a new list users handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle processes the user listing request.
// Route: GET /v1/dashboard/users
// All parameters are query params (no body binding).
func (h *Handler) Handle(c *echo.Context) error {
	// Read query params with defaults via Echo v5 generics.
	// QueryParamOr returns (T, error); we ignore parse errors (graceful defaults).
	limit, _ := echo.QueryParamOr[int](c, "limit", DefaultLimit)
	role, _ := echo.QueryParamOr[string](c, "role", "")
	status, _ := echo.QueryParamOr[string](c, "status", "")
	search, _ := echo.QueryParamOr[string](c, "search", "")
	cursor, _ := echo.QueryParamOr[string](c, "cursor", "")
	createdBefore, _ := echo.QueryParamOr[string](c, "created_before", "")
	createdAfter, _ := echo.QueryParamOr[string](c, "created_after", "")

	cmd := Command{
		Cursor:        cursor,
		Limit:         limit,
		Role:          role,
		Status:        status,
		Search:        search,
		CreatedBefore: createdBefore,
		CreatedAfter:  createdAfter,
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}
