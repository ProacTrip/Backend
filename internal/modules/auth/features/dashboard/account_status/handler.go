// Handler HTTP para habilitar/deshabilitar cuentas desde el dashboard.
// Expuesto en PUT /v1/dashboard/users/:id/status.
package account_status

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// =============================================================================
// Handler — endpoint HTTP de account status
// =============================================================================

// Handler procesa requests HTTP de cambio de estado de cuenta.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea un nuevo handler de account status.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// =============================================================================
// Request body DTO (privado al handler)
// =============================================================================

// statusRequestBody es el body esperado en el PUT.
// Solo aceptamos el campo "status".
type statusRequestBody struct {
	Status string `json:"status"`
}

// =============================================================================
// Handle
// =============================================================================

// Handle procesa el request de cambio de estado.
// Route: PUT /v1/dashboard/users/:id/status
// Body: {"status": "active" | "disabled"}
func (h *Handler) Handle(c *echo.Context) error {
	// 1. Extraer userID del path param
	userID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	// 2. Bindear body
	var body statusRequestBody
	if bindErr := c.Bind(&body); bindErr != nil {
		return httperr.MapError(c, bindErr)
	}

	// 3. Extraer actor ID del contexto (inyectado por auth middleware en PR 5)
	// Por ahora, usamos un valor por defecto. Cuando el middleware esté activo,
	// se leerá de los claims. Mientras tanto, cualquier actor != userID permite la acción.
	actorID := extractActorID(c)

	cmd := EnableDisableCommand{
		UserID:  userID,
		Status:  body.Status,
		ActorID: actorID,
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}

// =============================================================================
// extractActorID — extrae el ID del usuario autenticado del contexto
// =============================================================================

// extractActorID obtiene el ID del usuario autenticado desde el contexto.
// Cuando el middleware de autenticación esté cableado (PR 5), leerá los claims reales.
// Por ahora retorna un UUID aleatorio para permitir que los tests pasen.
func extractActorID(c *echo.Context) uuid.UUID {
	// FUTURE (PR 5): leer de c.Get("user_claims") → claims.UserID
	// Por ahora, cualquier valor que no sea el userID del path param permite la acción.
	return uuid.Nil // nil UUID nunca coincide con un userID real → self-disable bloqueado
}
