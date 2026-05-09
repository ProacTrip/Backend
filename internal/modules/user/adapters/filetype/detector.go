// Detector de tipo de archivo por magic bytes.
// Usa los primeros 512 bytes del archivo para determinar el MIME type real.
// Soporta 3 tipos de archivo aceptados: JPEG, PNG, PDF.
package filetype

import (
	"bytes"
	"fmt"

	"github.com/google/uuid"
	"lukechampine.com/blake3"
)

// =============================================================================
// Constantes — Magic Bytes
// =============================================================================

var magicPatterns = []struct {
	mime string
	mb   []byte
}{
	{mime: "image/jpeg", mb: []byte{0xFF, 0xD8, 0xFF}},
	{mime: "image/png", mb: []byte{0x89, 0x50, 0x4E, 0x47}},
	{mime: "application/pdf", mb: []byte{0x25, 0x50, 0x44, 0x46}},
}

// AcceptedTypes lista los 3 tipos MIME aceptados.
var AcceptedTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"application/pdf": true,
}

// =============================================================================
// DetectMimeType — Magic bytes detection (512 bytes header)
// =============================================================================

// DetectMimeType detecta el MIME type real de los primeros bytes del archivo.
func DetectMimeType(header []byte) (string, error) {
	if len(header) == 0 {
		return "", fmt.Errorf("empty header: cannot detect MIME type")
	}

	for _, p := range magicPatterns {
		if len(header) >= len(p.mb) && bytes.Equal(header[:len(p.mb)], p.mb) {
			return p.mime, nil
		}
	}

	return "", fmt.Errorf("unknown file type: magic bytes not recognized")
}

// =============================================================================
// IsAccepted — Checkea si un MIME type está en la lista de aceptados
// =============================================================================

// IsAccepted retorna true si el MIME type está en la lista de 3 tipos aceptados.
func IsAccepted(mime string) bool {
	return AcceptedTypes[mime]
}

// =============================================================================
// ContentHash — blake3 content dedup
// =============================================================================

// ContentHash calcula el hash blake3 del contenido completo del archivo.
func ContentHash(data []byte) string {
	hash := blake3.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// =============================================================================
// StorageKey — Genera la key de storage para R2
// =============================================================================

// StorageKey construye la key de storage R2 para el archivo raw de un documento.
// Formato: raw/{userID}/{docID}/original{ext}
func StorageKey(userID, docID uuid.UUID, mimeType string) string {
	ext := extFromMime(mimeType)
	return fmt.Sprintf("raw/%s/%s/original%s", userID.String(), docID.String(), ext)
}

// IsPDF returns true if the MIME type is application/pdf.
func IsPDF(mime string) bool {
	return mime == "application/pdf"
}

// IsImage returns true if the MIME type is an accepted image format.
func IsImage(mime string) bool {
	return mime == "image/jpeg" || mime == "image/png"
}

func extFromMime(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

// ExtFromMime es la versión exportada de extFromMime.
func ExtFromMime(mime string) string {
	return extFromMime(mime)
}
