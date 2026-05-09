// Comando para POST /v1/user/profile/medical/pending/resolve.
package resolve_medical_pending

import (
	"github.com/google/uuid"
)

// ValidActions contiene las acciones permitidas.
var ValidActions = map[string]bool{
	"accept": true,
	"reject": true,
	"custom": true,
}

// Command contiene los datos para resolver un conflicto médico pendiente.
type Command struct {
	UserID          string  `json:"-"`
	PendingUpdateID string  `json:"pending_update_id"`
	Action          string  `json:"action"`
	CustomValue     *string `json:"custom_value,omitzero"`
}

// Validate valida que los IDs sean UUIDs válidos y la acción sea permitida.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if _, err := uuid.Parse(c.PendingUpdateID); err != nil {
		return err
	}
	return nil
}
