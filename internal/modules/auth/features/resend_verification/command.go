package resend_verification

import (
	"fmt"
	"net/mail"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Command DTO para el reenvío de verificación de email.
// Recibe solo el email en el body JSON.
// Según la spec: siempre se retorna 200 para prevenir enumeración de usuarios.

type Command struct {
	Email string `json:"email"`
}

// Validate valida el comando antes de pasarlo al usecase.
// Retorna error wrapped con domain sentinel para que el handler
// pueda mapearlo a RFC 9457 via httperr.MapError.
func (c *Command) Validate() error {
	if c.Email == "" {
		return fmt.Errorf("%w: email is required", domain.ErrInvalidEmail)
	}
	if _, err := mail.ParseAddress(c.Email); err != nil {
		return fmt.Errorf("%w: invalid email format", domain.ErrInvalidEmail)
	}
	return nil
}
