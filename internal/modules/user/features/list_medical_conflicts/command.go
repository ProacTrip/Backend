// Comando para GET /v1/user/profile/medical-conflicts.
package list_medical_conflicts

import (
	"github.com/google/uuid"
)

// Command contiene los parámetros para listar conflictos médicos.
type Command struct {
	UserID string  `json:"-"`
	Status *string `query:"status"`
}

// Validate valida que el UserID sea un UUID válido.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	return nil
}
