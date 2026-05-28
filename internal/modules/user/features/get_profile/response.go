// DTO de respuesta para GET /v1/user/profile.
// Estructura flat alineada con docs/USER_API.md.
package get_profile

import (
	"github.com/google/uuid"
)

// GetProfileResponse es la respuesta completa del endpoint (flat, sin wrapper).
type GetProfileResponse struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	FirstName   *string   `json:"first_name,omitzero"`
	LastName    *string   `json:"last_name"`
	DateOfBirth *string   `json:"date_of_birth,omitzero"`
	Gender      *string   `json:"gender,omitzero"`
	Nationality *string   `json:"nationality,omitzero"`
	Phone       *string   `json:"phone,omitzero"`
	Bio         *string   `json:"bio,omitzero"`
	RoleName    string    `json:"role_name,omitzero"`
	AvatarURL *string `json:"avatar_url"`

	Location LocationResponse `json:"location"`
}

// LocationResponse representa el bloque location del perfil.
// Alineado con USER_API.md: solo currency y language.
type LocationResponse struct {
	Currency string `json:"currency,omitzero"`
	Language string `json:"language,omitzero"`
}
