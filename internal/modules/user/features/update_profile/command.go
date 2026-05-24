// Comando para PATCH /v1/user/profile.
// Todos los campos son punteros: nil = no tocar, valor = actualizar.
package update_profile

import (
	"regexp"
	"time"

	"github.com/google/uuid"
)

// validPhoneRegex valida números en formato E.164: ^\+[1-9]\d{1,14}$
// Requisito: + seguido de código de país (1-9, no 0), luego dígitos, máximo 15 dígitos total.
var validPhoneRegex = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

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

// IsValidPhone retorna true si el teléfono está en formato E.164 o es nil/vacío.
// nil = no tocar, "" = limpiar el campo, no-nil no-vacío = validar E.164.
func IsValidPhone(phone *string) bool {
	if phone == nil || *phone == "" {
		return true
	}
	return validPhoneRegex.MatchString(*phone)
}
