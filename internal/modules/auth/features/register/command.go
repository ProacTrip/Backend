package register

import (
	"fmt"
	"net/mail"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// DTOs del registro.
// Tokens van en cookies HTTP, no en el JSON.

type Command struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Validate valida los campos del comando de registro.
// Email requerido + formato RFC 5322. Password requerido + mínimo 8 caracteres.
func (c *Command) Validate() error {
	if c.Email == "" {
		return fmt.Errorf("%w: email is required", domain.ErrInvalidEmail)
	}
	if _, err := mail.ParseAddress(c.Email); err != nil {
		return fmt.Errorf("%w: invalid email format", domain.ErrInvalidEmail)
	}
	if c.Password == "" {
		return fmt.Errorf("%w: password is required", domain.ErrInvalidInput)
	}
	if len(c.Password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", domain.ErrPasswordTooShort)
	}
	return nil
}

// =============================================================================
// Response - DTO de salida del registro
// Según AUTH_API.md: solo message en JSON, tokens van en cookies

type Response struct {
	Message      string `json:"message"`
	AccessToken  string `json:"-"` // Para Set-Cookie, no en JSON
	RefreshToken string `json:"-"` // Para Set-Cookie, no en JSON
}
