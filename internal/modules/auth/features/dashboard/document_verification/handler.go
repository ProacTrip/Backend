// Handler HTTP para verificación de documentos desde el dashboard.
// Expuesto en GET /v1/dashboard/documents/:id/verification y
// PATCH /v1/dashboard/documents/:id/verification.
package document_verification

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

// =============================================================================
// Handler — endpoints HTTP de verificación de documentos
// =============================================================================

// Handler procesa requests HTTP de verificación de documentos.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea un nuevo handler de verificación de documentos.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// =============================================================================
// Request body DTO (privado al handler)
// =============================================================================

// verificationRequestBody es el body esperado en el PATCH.
// Solo aceptamos "status" (requerido) y "reason" (opcional, máx 500 chars).
type verificationRequestBody struct {
	Status string  `json:"status"`
	Reason *string `json:"reason"`
}

// =============================================================================
// HandleGet — GET /v1/dashboard/documents/:id/verification
// =============================================================================

// HandleGet procesa la consulta de estado de verificación.
// DV-REQ-1: retorna estado actual + historial completo.
// Permiso requerido: users:read (aplicado a nivel grupo en app.go).
func (h *Handler) HandleGet(c *echo.Context) error {
	// 1. Extraer documentID del path param
	documentID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	cmd := VerifyCommand{DocumentID: documentID}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusOK, resp)
}

// =============================================================================
// HandlePatch — PATCH /v1/dashboard/documents/:id/verification
// =============================================================================

// HandlePatch procesa el cambio de estado de verificación.
// DV-REQ-2: status debe ser verified|rejected|manual_review|suspicious.
// verified_by se extrae del PASETO claims, nunca del body.
// Permiso requerido: users:read + users:write (aplicado en app.go).
func (h *Handler) HandlePatch(c *echo.Context) error {
	// 1. Extraer documentID del path param
	documentID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	// 2. Bindear body
	var body verificationRequestBody
	if bindErr := c.Bind(&body); bindErr != nil {
		return httperr.MapError(c, bindErr)
	}

	// 3. Extraer actor ID de los claims del token PASETO
	actorID := extractActorID(c)

	cmd := VerifyStatusCommand{
		DocumentID: documentID,
		Status:     body.Status,
		Reason:     body.Reason,
		VerifiedBy: actorID,
	}

	resp, err := h.usecase.ExecuteUpdate(c.Request().Context(), cmd)
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
// Sigue el mismo patrón que account_status/handler.go.
func extractActorID(c *echo.Context) uuid.UUID {
	claims, err := sharedauth.GetAccessClaims(c)
	if err != nil {
		slog.Warn("extractActorID: no se pudieron obtener los claims del token",
			slog.String("error", err.Error()))
		return uuid.Nil
	}

	return claims.UserID
}
