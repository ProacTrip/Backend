// Handler HTTP para CRUD de feature limits del dashboard.
// Expuesto en:
//
//	GET    /v1/dashboard/users/:id/feature-limits
//	POST   /v1/dashboard/users/:id/feature-limits
//	DELETE /v1/dashboard/users/:id/feature-limits/:key
//	GET    /v1/dashboard/roles/:id/feature-limits
//	POST   /v1/dashboard/roles/:id/feature-limits
//	DELETE /v1/dashboard/roles/:id/feature-limits/:key
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
	resp, err := h.usecase.SetUserLimit(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}
	return c.JSON(http.StatusCreated, resp)
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

// =============================================================================
// Role Defaults Handlers
// =============================================================================

// HandleGetRoleDefaults lista los defaults de feature de un rol.
// Route: GET /v1/dashboard/roles/:id/feature-limits
func (h *Handler) HandleGetRoleDefaults(c *echo.Context) error {
	roleID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	cmd := GetRoleDefaultsCommand{RoleID: roleID}
	resp, err := h.usecase.GetRoleDefaults(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

// HandleSetRoleDefault crea o actualiza un default de feature para un rol.
// Route: POST /v1/dashboard/roles/:id/feature-limits
func (h *Handler) HandleSetRoleDefault(c *echo.Context) error {
	roleID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	var body limitRequestBody
	if bindErr := c.Bind(&body); bindErr != nil {
		return httperr.MapError(c, bindErr)
	}

	cmd := SetRoleDefaultCommand{
		RoleID:     roleID,
		FeatureKey: body.FeatureKey,
		LimitValue: body.LimitValue,
		Window:     body.Window,
	}
	resp, err := h.usecase.SetRoleDefault(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}
	return c.JSON(http.StatusCreated, resp)
}

// HandleDeleteRoleDefault elimina un default de feature de un rol.
// Route: DELETE /v1/dashboard/roles/:id/feature-limits/:key
func (h *Handler) HandleDeleteRoleDefault(c *echo.Context) error {
	roleID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	featureKey := c.Param("key")

	cmd := DeleteRoleDefaultCommand{
		RoleID:     roleID,
		FeatureKey: featureKey,
	}
	if delErr := h.usecase.DeleteRoleDefault(c.Request().Context(), cmd); delErr != nil {
		return httperr.MapError(c, delErr)
	}
	return c.NoContent(http.StatusNoContent)
}
