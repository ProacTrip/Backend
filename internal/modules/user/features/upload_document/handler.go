// Handler HTTP para POST /v1/user/profile/documents.
// Recibe el archivo como multipart/form-data.
// Handler es thin: extrae claims, parsea el form y delega toda validación al usecase.
package upload_document

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// Handler procesa POST /v1/user/profile/documents.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae user_claims, procesa el multipart form y delega al usecase.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*sharedauth.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	// 1. Rate limit — cheapest check, blocks spam before CPU/IO
	if err := h.usecase.CheckRateLimit(c.Request().Context(), claims.UserID.String()); err != nil {
		return httperr.MapError(c, err)
	}

	// 2. Content-Length pre-check — early rejection sin leer el body
	if contentLen := c.Request().Header.Get("Content-Length"); contentLen != "" {
		if size, parseErr := strconv.ParseInt(contentLen, 10, 64); parseErr == nil {
			maxForm := MaxFormSizeBytes()
			if size > maxForm {
				return httperr.MapError(c, fmt.Errorf("FILE_TOO_LARGE: %d bytes exceeds max form size %d bytes: %w",
					size, maxForm, domain.ErrFileTooLarge))
			}
		}
	}

	// 3. Extraer el archivo del form
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return httperr.MapError(c, domain.ErrInvalidFileType)
	}

	if fileHeader.Size == 0 {
		return httperr.MapError(c, domain.ErrFileTooLarge)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return httperr.MapError(c, fmt.Errorf("abrir archivo subido: %w", err))
	}
	defer file.Close()

	// 4. Leer el archivo completo (capped a MaxFormSizeBytes para prevenir OOM)
	// La validación de tamaño por MIME type la hace el usecase.
	lr := io.LimitReader(file, MaxFormSizeBytes()+1)
	fileBytes, readErr := io.ReadAll(lr)
	if readErr != nil {
		return httperr.MapError(c, fmt.Errorf("leer archivo: %w", readErr))
	}

	// 5. Obtener file_name del form
	fileName := c.FormValue("file_name")
	if fileName == "" {
		fileName = fileHeader.Filename
	}

	// 6. Construir comando y delegar al usecase
	cmd := UploadDocumentCommand{
		FileBytes: fileBytes,
		FileName:  fileName,
		FileSize:  int64(len(fileBytes)),
		UserID:    claims.UserID.String(),
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusAccepted, resp)
}
