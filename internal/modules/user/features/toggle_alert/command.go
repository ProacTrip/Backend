// Comando para PUT /v1/user/saved-searches/:search_id/alert.
package toggle_alert

import (
	"errors"

	"github.com/google/uuid"
)

// Command contiene el estado deseado de la alerta.
type Command struct {
	UserID   string `json:"-"`
	SearchID string `json:"-"`
	Enabled  bool   `json:"enabled"`
}

// Validate verifica que los UUIDs sean válidos.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if _, err := uuid.Parse(c.SearchID); err != nil {
		return errors.New("search_id inválido")
	}
	return nil
}
