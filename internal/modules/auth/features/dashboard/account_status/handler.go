// Handler HTTP para habilitar/deshabilitar cuentas desde el dashboard.
// Expuesto en PUT /v1/dashboard/users/:id/status.
package account_status

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
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

	// 3. Extraer actor ID de los claims del token PASETO
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

// extractActorID obtiene el ID del usuario autenticado desde los claims del token PASETO.
// Lee los claims inyectados por el middleware de autenticación en el contexto de Echo.
// Si no hay claims (sin autenticación), retorna uuid.Nil como fallback.
func extractActorID(c *echo.Context) uuid.UUID {
	claims, err := sharedauth.GetAccessClaims(c)
	if err != nil {
		slog.Warn("extractActorID: no se pudieron obtener los claims del token",
			slog.String("error", err.Error()))
		return uuid.Nil
	}

	return claims.UserID
}
