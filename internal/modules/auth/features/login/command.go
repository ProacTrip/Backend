package login

import (
	"fmt"
	"net/mail"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// DTOs del login según AUTH_API.md.

type Command struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Validate valida los campos del comando de login.
// Aplica las reglas de F1: usar domain sentinel errors con wrapping.
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

type Response struct {
	AccessToken  string        `json:"-"`
	RefreshToken string        `json:"-"`
	User         *UserResponse `json:"user"`
}

type UserResponse struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	RoleName      string    `json:"role_name"`
}
