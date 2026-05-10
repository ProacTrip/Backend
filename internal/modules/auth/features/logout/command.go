package logout

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// DTO de logout.
// LogoutAll: invalida todas las sesiones del usuario.

type Command struct {
	Token     string `json:"token"`
	LogoutAll bool   `json:"logout_all"`
}

// Validate valida los campos del comando de logout.
func (c *Command) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("%w: token is required", domain.ErrInvalidInput)
	}
	return nil
}
