// Comando para PUT /v1/user/profile/locale.
package update_locale

import (
	"github.com/google/uuid"
)

// Command contiene los campos localizables del perfil.
// Todos opcionales: nil = no actualizar.
type Command struct {
	UserID          string  `json:"-"`
	TimezoneName    *string `json:"timezone_name,omitzero"`
	LanguageCode    *string `json:"language_code,omitzero"`
	CurrencyCode    *string `json:"currency_code,omitzero"`
	CurrentLocation *string `json:"current_location,omitzero"`
}

func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	return nil
}
