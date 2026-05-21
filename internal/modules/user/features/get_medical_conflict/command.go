// Comando para GET /v1/user/profile/medical-conflicts/:conflict_id.
package get_medical_conflict

import (
	"github.com/google/uuid"
)

// Command contiene los parámetros para obtener un conflicto médico.
type Command struct {
	UserID     string `json:"-"`
	ConflictID string `json:"-"`
}

// Validate valida que UserID y ConflictID sean UUIDs válidos.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if _, err := uuid.Parse(c.ConflictID); err != nil {
		return err
	}
	return nil
}
