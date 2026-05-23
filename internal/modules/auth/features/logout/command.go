package logout

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// DTO de logout.

type Command struct {
	Token string `json:"token"`
}

// Validate valida los campos del comando de logout.
func (c *Command) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("%w: token is required", domain.ErrInvalidInput)
	}
	return nil
}
