// Handler HTTP para GET /v1/user/profile/documents.
// Thin handler: extrae claims y query params, delega al usecase,
// mapea domain.UserDocument → DocumentListItemResponse DTO.
package list_documents

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Handler procesa GET /v1/user/profile/documents.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae claims y query params, y delega al usecase.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	// Usar el helper del package shared/auth que hace el type assertion
	// internamente, evitando problemas de resolución de type alias
	// entre packages distintos en Go 1.26.
	claims, err := sharedauth.GetAccessClaims(c)
	if err != nil {
		return httperr.MapError(c, fmt.Errorf("extracting claims: %w", err))
	}

	statusFilter := c.QueryParam("status")
	docTypeFilter := c.QueryParam("document_type")

	docs, err := h.usecase.Execute(c.Request().Context(), claims.UserID.String(), statusFilter, docTypeFilter)
	if err != nil {
		return httperr.MapError(c, err)
	}

	// Mapear domain.UserDocument → DocumentListItemResponse DTO
	// Garantizar que nunca se serialice nil
	dtos := make([]DocumentListItemResponse, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		dtos = append(dtos, domainDocToListItem(doc))
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"documents": dtos,
	})
}

// domainDocToListItem convierte un domain.UserDocument al DTO de lista.
// Solo expone los 7 campos documentados en USER_API.md.
func domainDocToListItem(doc *domain.UserDocument) DocumentListItemResponse {
	return DocumentListItemResponse{
		ID:                 doc.ID.String(),
		FileName:           doc.FileName,
		DocumentType:       doc.DocumentType,
		OCRStatus:          string(doc.OCRStatus),
		OCRConfidence:      doc.OCRConfidence,
		VerificationStatus: string(doc.VerificationStatus),
		CreatedAt:          doc.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
