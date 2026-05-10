// Comando para PUT /v1/user/profile/notifications.
package update_notif_prefs

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Command contiene una preferencia de notificación individual.
// Channel y NotificationType son requeridos.
type Command struct {
	UserID           string `json:"-"`
	Channel          string `json:"channel"`
	NotificationType string `json:"notification_type"`
	Enabled          bool   `json:"enabled"`
}

// validNotifTypes define los tipos de notificación permitidos (canónicos).
var validNotifTypes = map[domain.NotificationType]bool{
	domain.NotifTypeBookingConfirmation: true,
	domain.NotifTypeFlightReminder:      true,
	domain.NotifTypePromotional:         true,
}

// Validate valida UserID (UUID), Channel (email|sms|websocket) y NotificationType.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}

	// Validar canal de notificación
	ch := domain.NotificationChannel(c.Channel)
	switch ch {
	case domain.NotifChannelEmail, domain.NotifChannelSMS, domain.NotifChannelWebSocket:
		// válido
	default:
		return fmt.Errorf("canal de notificación '%s' no válido (permitidos: email, sms, websocket): %w",
			c.Channel, domain.ErrInvalidEnum)
	}

	// Validar tipo de notificación
	if !validNotifTypes[domain.NotificationType(c.NotificationType)] {
		return fmt.Errorf("tipo de notificación '%s' no válido: %w",
			c.NotificationType, domain.ErrInvalidEnum)
	}

	return nil
}
