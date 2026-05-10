package authorize

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Command para la autorización OAuth — no tiene body,
// los parámetros vienen de la URL (path param provider).
type Command struct {
	Provider string // extraído de c.PathParam("provider")
}

// Validate valida los campos del comando de autorización OAuth.
func (c *Command) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("%w: provider is required", domain.ErrInvalidInput)
	}
	return nil
}
