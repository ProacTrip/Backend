// Errores específicos para el feature upload_avatar.
package upload_avatar

import (
	"errors"
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

var errFileNameRequired = errors.New("file_name: campo requerido")

var errMimeTypeRequired = errors.New("mime_type: campo requerido")

func errInvalidMimeType(mime string) error {
	return fmt.Errorf("%w: %s no está en la lista permitida (image/jpeg, image/png, image/webp)", domain.ErrInvalidMimeType, mime)
}

func errFileTooLarge(size int64) error {
	return fmt.Errorf("%w: %d bytes excede el máximo de %d bytes", domain.ErrFileTooLarge, size, MaxAvatarSizeBytes)
}
