// Comando para GET /v1/user/documents/:document_id/download.
// El handler extrae document_id de la URL y user_id de los claims del token.
package download_document

import (
	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Command contiene los parámetros para descargar un documento procesado.
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
