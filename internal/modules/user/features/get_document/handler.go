// Handler HTTP para GET /v1/user/profile/documents/:document_id.
// Thin handler: extrae claims y document_id, delega al usecase,
// mapea domain.UserDocument → DocumentDetailResponse DTO.
package get_document

import (
	"net/http"

	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Handler procesa GET /v1/user/profile/documents/:document_id.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae claims y document_id del path, y delega al usecase.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	documentID := c.Param("document_id")

	doc, err := h.usecase.Execute(c.Request().Context(), documentID, claims.UserID.String())
	if err != nil {
		return httperr.MapError(c, err)
	}

	dto := domainDocToDetail(doc)
	return c.JSON(http.StatusOK, dto)
}

// domainDocToDetail convierte un domain.UserDocument al DTO de detalle.
// Incluye verified_at + verified_by (si están disponibles).
func domainDocToDetail(doc *domain.UserDocument) DocumentDetailResponse {
	return DocumentDetailResponse{
		ID:                 doc.ID.String(),
		FileName:           doc.FileName,
		DocumentType:       doc.DocumentType,
		OCRStatus:          string(doc.OCRStatus),
		OCRConfidence:      doc.OCRConfidence,
		VerificationStatus: string(doc.VerificationStatus),
		CreatedAt:          doc.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
