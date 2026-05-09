// Handler HTTP para GET /v1/user/documents.
// Lista documentos del usuario con filtros opcionales por status y tipo.
package list_documents

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// ListDocRepo es el puerto para listar documentos.
type ListDocRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error)
	GetByUserIDFiltered(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error)
}

// Handler procesa GET /v1/user/documents.
type Handler struct {
	repo ListDocRepo
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(repo ListDocRepo) *Handler {
	return &Handler{repo: repo}
}

// Handle lista documentos con filtros opcionales.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	userID, err := uuid.Parse(claims.UserID.String())
	if err != nil {
		return httperr.MapError(c, err)
	}

	statusFilter := c.QueryParam("status")
	docTypeFilter := c.QueryParam("document_type")

	var docs []*domain.UserDocument

	if statusFilter != "" || docTypeFilter != "" {
		docs, err = h.repo.GetByUserIDFiltered(c.Request().Context(), userID, domain.OCRStatus(statusFilter), docTypeFilter)
	} else {
		docs, err = h.repo.GetByUserID(c.Request().Context(), userID)
	}

	if err != nil {
		return httperr.MapError(c, err)
	}

	if docs == nil {
		docs = []*domain.UserDocument{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"documents": docs,
	})
}
