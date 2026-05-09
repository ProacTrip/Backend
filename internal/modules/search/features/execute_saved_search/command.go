// Comando para POST /v1/search/execute_saved.
// Recibe el ID de una búsqueda guardada para ejecutarla.
package execute_saved_search

import (
	"fmt"

	"github.com/google/uuid"
)

// Command es el input DTO para ejecutar una búsqueda guardada.
type Command struct {
	SavedSearchID uuid.UUID `json:"saved_search_id"`

	// Resolved from cookie auth by the handler — never from JSON body.
	UserID uuid.UUID `json:"-"`
}

// Validate checks that saved_search_id is a valid non-nil UUID.
func (c *Command) Validate() error {
	if c.SavedSearchID == uuid.Nil {
		return fmt.Errorf("saved_search_id es requerido")
	}
	if c.UserID == uuid.Nil {
		return fmt.Errorf("user_id es requerido")
	}
	return nil
}
