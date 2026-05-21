// Comando para PUT /v1/user/profile.
// Todos los campos son punteros: nil = no tocar, valor = actualizar.
package update_profile

import (
	"time"

	"github.com/google/uuid"
)

// Command contiene los campos actualizables del perfil.
// Todos los campos son opcionales (*pointer = nil significa "no actualizar").
type Command struct {
	UserID      string     `json:"-"`
	FirstName   *string    `json:"first_name"`
	LastName    *string    `json:"last_name"`
	DateOfBirth *time.Time `json:"date_of_birth,omitzero"`
	Gender      *string    `json:"gender,omitzero"`
	Nationality *string    `json:"nationality,omitzero"`
	Phone       *string    `json:"phone,omitzero"`
	Bio       *string `json:"bio,omitzero"`
	Language  *string `json:"language,omitzero"`
	Currency  *string `json:"currency,omitzero"`
}

// Validate verifica que UserID sea un UUID válido.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	return nil
}
