// Handler HTTP para CRUD de permission overrides del dashboard.
// Expuesto en:
//
//	GET    /v1/dashboard/users/:id/permission-overrides
//	POST   /v1/dashboard/users/:id/permission-overrides
//	DELETE /v1/dashboard/users/:id/permission-overrides/:overrideId
package permission_overrides

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// =============================================================================
// Handler — endpoints HTTP de permission overrides
// =============================================================================

// Handler procesa requests HTTP de permission overrides.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea un nuevo handler de permission overrides.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// =============================================================================
// Request body (privado al handler)
// =============================================================================

type overrideRequestBody struct {
	PermissionID string     `json:"permission_id"`
	Granted      bool       `json:"granted"`
	Reason       string     `json:"reason"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

// =============================================================================
// List Overrides
// =============================================================================

// HandleListOverrides lista todos los overrides de un usuario.
// Route: GET /v1/dashboard/users/:id/permission-overrides
func (h *Handler) HandleListOverrides(c *echo.Context) error {
	userID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	cmd := ListOverridesCommand{UserID: userID}
	resp, err := h.usecase.ListOverrides(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}
	return c.JSON(http.StatusOK, resp)
}

// =============================================================================
// Create Override
// =============================================================================

// HandleCreateOverride crea un override de permiso para un usuario.
// Route: POST /v1/dashboard/users/:id/permission-overrides
func (h *Handler) HandleCreateOverride(c *echo.Context) error {
	userID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	var body overrideRequestBody
	if bindErr := c.Bind(&body); bindErr != nil {
		return httperr.MapError(c, bindErr)
	}

	permissionID, parseErr := uuid.Parse(body.PermissionID)
	if parseErr != nil {
		return httperr.MapError(c, parseErr)
	}

	actorID := extractActorID(c)

	cmd := CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permissionID,
		Granted:      body.Granted,
		Reason:       body.Reason,
		ExpiresAt:    body.ExpiresAt,
		ActorID:      actorID,
	}
	resp, err := h.usecase.CreateOverride(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}
	return c.JSON(http.StatusCreated, resp)
}

// =============================================================================
// Delete Override
// =============================================================================

// HandleDeleteOverride elimina un override de permiso.
// Route: DELETE /v1/dashboard/users/:id/permission-overrides/:overrideId
func (h *Handler) HandleDeleteOverride(c *echo.Context) error {
	overrideID, err := echo.PathParam[uuid.UUID](c, "overrideId")
	if err != nil {
		return httperr.MapError(c, err)
	}

	actorID := extractActorID(c)

	cmd := DeleteOverrideCommand{
		OverrideID: overrideID,
		ActorID:    actorID,
	}
	if delErr := h.usecase.DeleteOverride(c.Request().Context(), cmd); delErr != nil {
		return httperr.MapError(c, delErr)
	}
	return c.NoContent(http.StatusNoContent)
}

// =============================================================================
// extractActorID — extrae el ID del usuario autenticado del contexto
// =============================================================================

// extractActorID obtiene el ID del usuario autenticado desde el contexto.
// FUTURE (PR 5): leer de c.Get("user_claims") → claims.UserID
func extractActorID(c *echo.Context) uuid.UUID {
	return uuid.Nil
}
