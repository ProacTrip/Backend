// Comando para POST /v1/user/profile/avatar.
// Contiene los campos necesarios para generar una URL prefirmada de R2.
package upload_avatar

import (
	"github.com/google/uuid"
)

// AcceptedMimeTypes son los tipos MIME aceptados para avatares.
var AcceptedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// MaxAvatarSizeBytes es el tamaño máximo de archivo para avatares (5 MB).
const MaxAvatarSizeBytes int64 = 5242880

// DefaultTTLMinutes es el TTL de la URL prefirmada por defecto.
const DefaultTTLMinutes = 15

// Command contiene los campos para solicitar una URL de subida de avatar.
type Command struct {
	UserID     string  `json:"-"`
	FileName   string  `json:"file_name"`
	MimeType   string  `json:"mime_type"`
	FileSize   *int64  `json:"file_size,omitzero"`
	TTLMinutes *int    `json:"ttl_minutes,omitzero"`
}

// Validate verifica que los campos del comando sean válidos.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if c.FileName == "" {
		return errFileNameRequired
	}
	if c.MimeType == "" {
		return errMimeTypeRequired
	}
	if !AcceptedMimeTypes[c.MimeType] {
		return errInvalidMimeType(c.MimeType)
	}
	if c.FileSize != nil && *c.FileSize > MaxAvatarSizeBytes {
		return errFileTooLarge(*c.FileSize)
	}
	return nil
}
