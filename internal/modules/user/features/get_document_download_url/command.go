// Comando para GET /v1/user/profile/documents/:document_id/download-url.
// El handler extrae document_id de la URL y user_id de los claims del token.
package get_document_download_url

import (
	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Command contiene los parámetros para generar una URL de descarga.
type Command struct {
	DocumentID uuid.UUID `json:"-"`
	UserID     uuid.UUID `json:"-"`
}

// Validate verifica que los UUIDs no sean nil.
func (c *Command) Validate() error {
	if c.DocumentID == uuid.Nil {
		return domain.ErrDocumentNotFound
	}
	if c.UserID == uuid.Nil {
		return domain.ErrDocumentNotFound
	}
	return nil
}
