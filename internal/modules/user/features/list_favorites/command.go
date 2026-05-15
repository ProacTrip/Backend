// Comando para GET /v1/user/favorites.
package list_favorites

import (
	"github.com/google/uuid"
)

// Command contiene el user_id del token y el filtro opcional por entity_type.
type Command struct {
	UserID           string  `json:"-"`
	EntityTypeFilter *string // opcional — query param entity_type
}

// Validate valida que el UserID sea un UUID válido.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	return nil
}
