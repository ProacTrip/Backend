package verify_email

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Request body para verificación de email.

// Command representa el request body del endpoint verify-email
type Command struct {
	Token string `json:"token"`
}

// Validate valida los campos del comando de verificación de email.
func (c *Command) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("%w: token is required", domain.ErrInvalidInput)
	}
	return nil
}
