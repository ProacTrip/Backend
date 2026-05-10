// Domain: Preferencias de notificación del usuario.
// Define canales y tipos de notificación.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Enums
// =============================================================================

// NotificationChannel representa el canal de notificación.
type NotificationChannel string

const (
	NotifChannelEmail     NotificationChannel = "email"
	NotifChannelSMS       NotificationChannel = "sms"
	NotifChannelWebSocket NotificationChannel = "websocket"
)

// NotificationType representa el tipo de notificación.
type NotificationType string

const (
	// Tipos canónicos de notificación (USER_API.md).
	NotifTypeBookingConfirmation NotificationType = "booking_confirmation"
	NotifTypeFlightReminder      NotificationType = "flight_reminder"
	NotifTypePromotional         NotificationType = "promotional"
)

// =============================================================================
// NotificationPreference — Preferencia de notificación
// =============================================================================

// NotificationPreference representa una preferencia de notificación individual.
// Un usuario puede tener múltiples preferencias, una por canal+tipo.
// Alineado con la migración user_notification_preferences.
type NotificationPreference struct {
	ID               uuid.UUID          `json:"id"`
	UserID           uuid.UUID          `json:"user_id"`
	Channel          NotificationChannel `json:"channel"`
	NotificationType NotificationType   `json:"notification_type"`
	Enabled          bool               `json:"enabled"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
}

// NewNotificationPreference crea una nueva preferencia de notificación habilitada.
func NewNotificationPreference(userID uuid.UUID, channel NotificationChannel, notifType NotificationType) *NotificationPreference {
	now := time.Now()
	return &NotificationPreference{
		ID:               uuid.Must(uuid.NewV7()),
		UserID:           userID,
		Channel:          channel,
		NotificationType: notifType,
		Enabled:          true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// Toggle alterna el estado habilitado/deshabilitado.
func (np *NotificationPreference) Toggle() {
	np.Enabled = !np.Enabled
	np.UpdatedAt = time.Now()
}

// Disable deshabilita la notificación.
func (np *NotificationPreference) Disable() {
	np.Enabled = false
	np.UpdatedAt = time.Now()
}

// Enable habilita la notificación.
func (np *NotificationPreference) Enable() {
	np.Enabled = true
	np.UpdatedAt = time.Now()
}
