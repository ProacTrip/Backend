// Comando para PUT /v1/user/profile/notifications.
package update_notif_prefs

import (
	"github.com/google/uuid"
)

// Command contiene una preferencia de notificación individual.
// Channel y NotificationType son requeridos.
type Command struct {
	UserID           string `json:"-"`
	Channel          string `json:"channel"`
	NotificationType string `json:"notification_type"`
	Enabled          bool   `json:"enabled"`
}

func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	return nil
}
