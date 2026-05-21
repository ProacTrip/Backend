// Handler HTTP para CRUD de feature limits del dashboard.
// Expuesto en:
//
//	GET    /v1/dashboard/users/:id/feature-limits
//	POST   /v1/dashboard/users/:id/feature-limits
//	DELETE /v1/dashboard/users/:id/feature-limits/:key
package feature_limits

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// =============================================================================
// Handler — endpoints HTTP de feature limits
// =============================================================================

// Handler procesa requests HTTP de feature limits.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea un nuevo handler de feature limits.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// =============================================================================
// Request body (privado al handler)
// =============================================================================

type limitRequestBody struct {
	FeatureKey string `json:"feature_key"`
	LimitValue *int   `json:"limit_value"`
	Window     string `json:"window"`
}

// =============================================================================
// User Limits Handlers
// =============================================================================

// HandleGetUserLimits lista los límites de feature de un usuario.
// Route: GET /v1/dashboard/users/:id/feature-limits
func (h *Handler) HandleGetUserLimits(c *echo.Context) error {
	userID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	cmd := GetUserLimitsCommand{UserID: userID}
	resp, err := h.usecase.GetUserLimits(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

// HandleSetUserLimit crea o actualiza un límite de feature para un usuario.
// Route: POST /v1/dashboard/users/:id/feature-limits
func (h *Handler) HandleSetUserLimit(c *echo.Context) error {
	userID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	var body limitRequestBody
	if bindErr := c.Bind(&body); bindErr != nil {
		return httperr.MapError(c, bindErr)
	}

	cmd := SetUserLimitCommand{
		UserID:     userID,
		FeatureKey: body.FeatureKey,
		LimitValue: body.LimitValue,
		Window:     body.Window,
	}
	resp, isCreated, err := h.usecase.SetUserLimit(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	statusCode := http.StatusOK
	if isCreated {
		statusCode = http.StatusCreated
	}
	return c.JSON(statusCode, resp)
}

// HandleDeleteUserLimit elimina un límite de feature de un usuario.
// Route: DELETE /v1/dashboard/users/:id/feature-limits/:key
func (h *Handler) HandleDeleteUserLimit(c *echo.Context) error {
	userID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	featureKey := c.Param("key")

	cmd := DeleteUserLimitCommand{
		UserID:     userID,
		FeatureKey: featureKey,
	}
	if delErr := h.usecase.DeleteUserLimit(c.Request().Context(), cmd); delErr != nil {
		return httperr.MapError(c, delErr)
	}
	return c.NoContent(http.StatusNoContent)
}
