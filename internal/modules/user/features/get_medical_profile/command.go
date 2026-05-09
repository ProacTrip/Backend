// Comando para GET /v1/user/profile/medical.
package get_medical_profile

import (
	"github.com/google/uuid"
)

// Command contiene el user_id extraído del token de autenticación.
type Command struct {
	UserID string `json:"-"`
}

// Validate valida que el UserID sea un UUID válido.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	return nil
}
