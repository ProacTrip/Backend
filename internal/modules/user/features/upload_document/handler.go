// Handler HTTP para POST /v1/user/documents.
// Recibe el archivo como multipart/form-data.
// Checks Content-Length upfront, magic bytes sync, size capping via LimitReader.
package upload_document

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/adapters/filetype"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// Handler procesa POST /v1/user/documents.
type Handler struct {
	usecase *UseCase
}

// NewHandler crea una nueva instancia del handler.
func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase}
}

// Handle extrae user_claims, procesa el multipart form, valida y sube el documento.
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
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

	// 4. Leer primeros 512 bytes para magic bytes
	headerBytes := make([]byte, 512)
	n, err := io.ReadFull(file, headerBytes)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return httperr.MapError(c, fmt.Errorf("leer header del archivo: %w", err))
	}
	headerBytes = headerBytes[:n]

	// 5. Magic bytes check sincrónico
	detectedMime, err := filetype.DetectMimeType(headerBytes)
	if err != nil || !filetype.IsAccepted(detectedMime) {
		return httperr.MapError(c, domain.ErrInvalidFileType)
	}

	// 6. Determinar el límite de tamaño según MIME type
	maxSize := MaxSizeForMIME(detectedMime)

	// 7. Seek al inicio + leer todo con LimitReader para cap el stream
	seeker, ok := file.(io.Seeker)
	if !ok {
		return httperr.MapError(c, fmt.Errorf("file does not support seeking"))
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return httperr.MapError(c, fmt.Errorf("seek archivo: %w", err))
	}

	lr := io.LimitReader(file, maxSize+1) // +1 para detectar truncation
	fileBytes, readErr := io.ReadAll(lr)
	if readErr != nil {
		return httperr.MapError(c, fmt.Errorf("leer archivo: %w", readErr))
	}

	// 8. Truncation check: si leímos > maxSize, se excedió el límite por MIME
	actualSize := int64(len(fileBytes))
	if actualSize > maxSize {
		return httperr.MapError(c, fmt.Errorf("FILE_TOO_LARGE: %d bytes exceeds max %d bytes for %s: %w",
			actualSize, maxSize, detectedMime, domain.ErrFileTooLarge))
	}

	// 9. Obtener file_name del form
	fileName := c.FormValue("file_name")
	if fileName == "" {
		fileName = fileHeader.Filename
	}

	// 10. Construir comando
	cmd := UploadDocumentCommand{
		FileBytes: fileBytes,
		FileName:  fileName,
		FileSize:  actualSize,
		MimeType:  detectedMime,
		UserID:    claims.UserID.String(),
	}

	// 11. Ejecutar caso de uso
	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	return c.JSON(http.StatusAccepted, resp)
}


