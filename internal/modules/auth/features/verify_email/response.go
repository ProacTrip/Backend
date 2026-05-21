package verify_email

import "github.com/google/uuid"

// DTOs de respuesta según AUTH_API.md.

// Response es la respuesta del endpoint verify-email
type Response struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"-"` // Para Set-Cookie, no en JSON
	RefreshToken string       `json:"-"` // Para Set-Cookie, no en JSON
}

// UserResponse contiene los datos del usuario verificado
type UserResponse struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	RoleName string    `json:"role_name"`
}
