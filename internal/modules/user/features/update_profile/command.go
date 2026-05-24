// Comando para PATCH /v1/user/profile.
// Todos los campos son punteros: nil = no tocar, valor = actualizar.
package update_profile

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// validPhoneRegex valida números en formato E.164: ^\+[1-9]\d{1,14}$
// Requisito: + seguido de código de país (1-9, no 0), luego dígitos, máximo 15 dígitos total.
var validPhoneRegex = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

// =============================================================================
// DateOnly — Tipo que acepta formato YYYY-MM-DD como JSON y lo parsea a time.Time
// =============================================================================
// Go's default time.Time JSON unmarshaling espera RFC 3339 ("2006-01-02T15:04:05Z07:00").
// El frontend envía date_of_birth como "1990-05-15" (YYYY-MM-DD, ISO 8601 date-only).
// DateOnly implementa json.Unmarshaler para aceptar este formato.
type DateOnly time.Time

// UnmarshalJSON parsea "YYYY-MM-DD" a DateOnly (wrapping time.Time).
func (d *DateOnly) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}
	*d = DateOnly(t)
	return nil
}

// MarshalJSON serializa DateOnly como "YYYY-MM-DD".
func (d DateOnly) MarshalJSON() ([]byte, error) {
	s := time.Time(d).Format("2006-01-02")
	return json.Marshal(s)
}

// Time convierte DateOnly a time.Time.
func (d *DateOnly) Time() time.Time {
	if d == nil {
		return time.Time{}
	}
	return time.Time(*d)
}

// =============================================================================
// Command
// =============================================================================

// Command contiene los campos actualizables del perfil.
// Todos los campos son opcionales (*pointer = nil significa "no actualizar").
type Command struct {
	UserID      string     `json:"-"`
	FirstName   *string    `json:"first_name"`
	LastName    *string    `json:"last_name"`
	DateOfBirth *DateOnly  `json:"date_of_birth,omitzero"`
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
