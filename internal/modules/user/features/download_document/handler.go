// Handler HTTP para GET /v1/user/documents/:document_id/download.
// Streamea el archivo procesado desde R2 al cliente.
// Delega la lógica de negocio al usecase.
package download_document

import (
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// Handler procesa GET /v1/user/documents/:document_id/download.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle streamea el archivo desde R2 a través del usecase.
func (h *Handler) Handle(c *echo.Context) error {
	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	docID, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), Command{
		DocumentID: docID,
		UserID:     claims.UserID,
	})
	if err != nil {
		return httperr.MapError(c, err)
	}
	defer resp.Reader.Close()

	// Setear headers de respuesta
	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, resp.FileName))
	c.Response().Header().Set("Content-Type", resp.MimeType)
	c.Response().Header().Set("Cache-Control", "private, max-age=300")

	// Streamear el archivo
	if _, err := io.Copy(c.Response(), resp.Reader); err != nil {
		return fmt.Errorf("stream document: %w", err)
	}

	return nil
}
