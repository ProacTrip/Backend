// Comando para POST /v1/user/profile/avatar/confirm.
package confirm_avatar_upload

import (
	"errors"

	"github.com/google/uuid"
)

// Command contiene el storage_key del avatar subido.
type Command struct {
	UserID     string `json:"-"`
	StorageKey string `json:"storage_key"`
}

// Validate verifica que el comando sea válido.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if c.StorageKey == "" {
		return errors.New("storage_key: campo requerido")
	}
	return nil
}
