// Command y Request para upload de documentos.
// El handler usa multipart/form-data — NO usa c.Bind().
// Los campos se extraen manualmente desde FormFile y FormValue.
package upload_document

import (
	"os"
	"strconv"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/filetype"
)

// =============================================================================
// Configuración de tamaño desde env
// =============================================================================

// MaxDocumentSizeBytes retorna el tamaño máximo en bytes para PDFs.
// Configurable via DOCUMENT_MAX_SIZE_MB (default 20 MB).
func MaxDocumentSizeBytes() int64 {
	return parseEnvMB("DOCUMENT_MAX_SIZE_MB", 20)
}

// MaxImageSizeBytes retorna el tamaño máximo en bytes para imágenes.
// Configurable via IMAGE_MAX_SIZE_MB (default 10 MB).
func MaxImageSizeBytes() int64 {
	return parseEnvMB("IMAGE_MAX_SIZE_MB", 10)
}

// MaxSizeForMIME retorna el tamaño máximo según tipo MIME.
func MaxSizeForMIME(mime string) int64 {
	if filetype.IsPDF(mime) {
		return MaxDocumentSizeBytes()
	}
	return MaxImageSizeBytes()
}

// MaxFormSizeBytes retorna el máximo de ambos límites (para pre-check de Content-Length).
func MaxFormSizeBytes() int64 {
	doc := MaxDocumentSizeBytes()
	img := MaxImageSizeBytes()
	if doc > img {
		return doc
	}
	return img
}

// MaxDocumentsPerUser retorna el límite de docs por usuario.
// Configurable via DOCUMENT_MAX_PER_USER (default 5).
func MaxDocumentsPerUser() int {
	return int(parseEnvInt("DOCUMENT_MAX_PER_USER", 5))
}

// RateLimitMax retorna el límite de subidas por ventana.
// Configurable via DOCUMENT_UPLOAD_RATE_LIMIT (default 10).
func RateLimitMax() int {
	return int(parseEnvInt("DOCUMENT_UPLOAD_RATE_LIMIT", 10))
}

// RateLimitWindowSecs retorna la ventana de rate limit en segundos.
// Configurable via DOCUMENT_UPLOAD_RATE_WINDOW (default 60).
func RateLimitWindowSecs() int {
	return int(parseEnvInt("DOCUMENT_UPLOAD_RATE_WINDOW", 60))
}

func parseEnvMB(key string, defaultMB int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultMB * 1024 * 1024
	}
	mb, err := strconv.ParseInt(v, 10, 64)
	if err != nil || mb <= 0 {
		return defaultMB * 1024 * 1024
	}
	return mb * 1024 * 1024
}

func parseEnvInt(key string, defaultVal int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}

// =============================================================================
// UploadDocumentCommand — Datos del documento a subir
// =============================================================================

// UploadDocumentCommand contiene los datos extraídos del multipart form.
type UploadDocumentCommand struct {
	FileBytes []byte // archivo completo leído en el handler
	FileName  string // nombre del archivo
	FileSize  int64  // tamaño real en bytes
	UserID    string // user_id desde el token de auth
	MimeType  string `json:"-"` // detectado por el usecase, no viene del handler
}
