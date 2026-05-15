// Comando para DELETE /v1/user/favorites/:favorite_id.
package delete_favorite

import (
	"github.com/google/uuid"
)

// Command contiene el favorite_id extraído del path y el user_id del token.
type Command struct {
	FavoriteID string `json:"-"`
	UserID     string `json:"-"`
}

// Validate valida que FavoriteID sea un UUID válido.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.FavoriteID); err != nil {
		return err
	}
	return nil
}
