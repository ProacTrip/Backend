package callback

import "github.com/google/uuid"

// Command recibe code y state de los query params del callback de Google.
type Command struct {
	Code  string // código de autorización de Google
	State string // state token PASETO generado en el authorize
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
