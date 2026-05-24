// Handler HTTP para reprocesamiento OCR de documentos desde el dashboard.
// Expuesto en POST /v1/dashboard/documents/:id/reprocess → 202 Accepted.
package document_reprocess

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

// =============================================================================
// Handler — endpoint HTTP de reprocesamiento OCR
// =============================================================================

// Handler procesa requests HTTP de reprocesamiento OCR de documentos.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea un nuevo handler de reprocesamiento de documentos.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// =============================================================================
// Handle — POST /v1/dashboard/documents/:id/reprocess
// =============================================================================

// Handle procesa el request de reprocesamiento OCR.
// DR-REQ-1: retorna 202 Accepted, el pipeline OCR se ejecuta asíncronamente.
// Permiso requerido: users:read + users:write (aplicado en app.go).
func (h *Handler) Handle(c *echo.Context) error {
	// 1. Extraer documentID del path param
	documentID, err := echo.PathParam[uuid.UUID](c, "id")
	if err != nil {
		return httperr.MapError(c, err)
	}

	// 2. Extraer actor ID de los claims del token PASETO
	actorID := extractActorID(c)

	cmd := ReprocessCommand{
		DocumentID: documentID,
		ActorID:    actorID,
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusAccepted, resp)
}

// =============================================================================
// extractActorID — extrae el ID del usuario autenticado del contexto
// =============================================================================

// extractActorID obtiene el ID del usuario autenticado desde los claims del token PASETO.
// Sigue el mismo patrón que account_status/handler.go y document_verification/handler.go.
func extractActorID(c *echo.Context) uuid.UUID {
	claims, err := sharedauth.GetAccessClaims(c)
	if err != nil {
		slog.Warn("extractActorID: no se pudieron obtener los claims del token",
			slog.String("error", err.Error()))
		return uuid.Nil
	}

	return claims.UserID
}
