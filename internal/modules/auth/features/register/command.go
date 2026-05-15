package register

import (
	"fmt"
	"net/mail"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// DTOs del registro.
// Tokens van en cookies HTTP, no en el JSON.

type Command struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name,omitempty"` // Opcional: se pasa al perfil y al email de verificación
}

// Validate valida los campos del comando de registro.
// Email requerido + formato RFC 5322. Password requerido + mínimo 8 caracteres
// y formato seguro (mayúscula, minúscula, dígito, carácter especial).
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
	if err := validatePasswordFormat(c.Password); err != nil {
		return err
	}
	return nil
}

// validatePasswordFormat verifica que la contraseña cumpla con los requisitos
// de complejidad: al menos una mayúscula, una minúscula, un dígito y un
// carácter especial.
func validatePasswordFormat(password string) error {
	var (
		hasUpper   bool
		hasLower   bool
		hasDigit   bool
		hasSpecial bool
	)
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return fmt.Errorf("%w: la contraseña debe contener al menos una mayúscula, una minúscula, un dígito y un carácter especial",
			domain.ErrInvalidPassword)
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
