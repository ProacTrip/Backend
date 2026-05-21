// Comando para POST /v1/user/profile/medical-conflicts/:conflict_id/resolve.
package resolve_medical_conflict

import (
	"github.com/google/uuid"
)

// ValidActions contiene las acciones permitidas.
var ValidActions = map[string]bool{
	"accept": true,
	"reject": true,
	"custom": true,
}

// Command contiene los datos para resolver un conflicto médico.
type Command struct {
	UserID     string  `json:"-"`
	ConflictID string  `json:"-"`
	Action     string  `json:"action"`
	Value      *string `json:"value,omitzero"`
}

// Validate valida que los IDs sean UUIDs válidos y la acción sea permitida.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if _, err := uuid.Parse(c.ConflictID); err != nil {
		return err
	}
	return nil
}
