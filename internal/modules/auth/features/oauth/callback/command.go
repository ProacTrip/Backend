package callback

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Command recibe los parámetros del callback OAuth.
type Command struct {
	ProviderCode string `json:"code"` // código de autorización del proveedor OAuth
	State        string `json:"state"` // state token PASETO generado en el authorize
	Provider     string `json:"-"` // provider code poblado por el handler desde el path param
}

// Validate valida los campos del comando de callback OAuth.
func (c *Command) Validate() error {
	if c.ProviderCode == "" {
		return fmt.Errorf("%w: authorization code is required", domain.ErrOAuthCodeMissing)
	}
	if c.State == "" {
		return fmt.Errorf("%w: state token is required", domain.ErrOAuthStateMissing)
	}
	return nil
}

// Response retorna los tokens y datos del usuario para el redirect.
// Los tokens van en cookies HTTP, no en el JSON.
type Response struct {
	AccessToken  string        `json:"-"` // para Set-Cookie
	RefreshToken string        `json:"-"` // para Set-Cookie
	User         *UserResponse `json:"user"`
}

// UserResponse contiene los datos públicos del usuario.
type UserResponse struct {
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	RoleName     string    `json:"role_name"`
}
