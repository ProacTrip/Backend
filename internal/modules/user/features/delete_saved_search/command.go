// Comando para DELETE /v1/user/saved-searches/:search_id.
package delete_saved_search

import (
	"github.com/google/uuid"
)

// Command contiene el search_id extraído del path y el user_id del token.
type Command struct {
	SearchID string `json:"-"`
	UserID   string `json:"-"`
}

// Validate valida que SearchID sea un UUID válido.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.SearchID); err != nil {
		return err
	}
	return nil
}
