// Domain: Entidades y tipos de dominio para notificaciones.
// Define la estructura de una notificación y sus estados.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Tipos de notificación - Alineados con migración 001_notifications.sql
// =============================================================================

// NotificationType representa el tipo de notificación
type NotificationType string

const (
	NotificationTypeTransactional NotificationType = "transactional"
	NotificationTypeMarketing     NotificationType = "marketing"
	NotificationTypeSystem        NotificationType = "system"
)

// NotificationChannel representa el canal de delivery (alineado con migration)
type NotificationChannel string

const (
	NotificationChannelEmail     NotificationChannel = "email"
	NotificationChannelSMS       NotificationChannel = "sms"
	NotificationChannelWebsocket NotificationChannel = "websocket"
)

// NotificationStatus representa el estado de entrega (alineado con migration)
type NotificationStatus string

const (
	NotificationStatusPending    NotificationStatus = "pending"
	NotificationStatusProcessing NotificationStatus = "processing"
	NotificationStatusSent       NotificationStatus = "sent"
	NotificationStatusDelivered  NotificationStatus = "delivered"
	NotificationStatusFailed     NotificationStatus = "failed"
	NotificationStatusBounced    NotificationStatus = "bounced"
)

// Notification representa una notificación completa (alineada con migration)
type Notification struct {
	ID                uuid.UUID           `json:"id"`
	UserID            uuid.UUID           `json:"user_id"`
	TemplateCode      string              `json:"template_code,omitempty"` // Resend template ID
	Type              NotificationType    `json:"type"`                    // transactional/marketing/system
	Channel           NotificationChannel `json:"channel"`                 // email/sms/websocket (NOT NULL)
	Subject           string              `json:"subject,omitempty"`
	Content           string              `json:"content"` // NOT NULL
	Data              map[string]any      `json:"data,omitempty"`
	Status            NotificationStatus  `json:"status"` // pending/processing/sent/delivered/failed/bounced
	SentAt            *time.Time          `json:"sent_at,omitzero"`
	DeliveredAt       *time.Time          `json:"delivered_at,omitzero"`
	OpenedAt          *time.Time          `json:"opened_at,omitzero"`
	ErrorMessage      string              `json:"error_message,omitempty"`
	ProviderMessageID string              `json:"provider_message_id,omitempty"` // Resend message ID
	Metadata          map[string]any      `json:"metadata,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

// =============================================================================
// Métodos de dominio
// =============================================================================

// NewNotification crea una nueva notificación
// Channel y Content son OBLIGATORIOS según el schema
func NewNotification(
	userID uuid.UUID,
	channel NotificationChannel,
	content string,
	nType NotificationType,
	subject string,
	templateCode string,
	data map[string]any,
) *Notification {
	now := time.Now()
	return &Notification{
		ID:           uuid.Must(uuid.NewV7()),
		UserID:       userID,
		TemplateCode: templateCode,
		Type:         nType,
		Channel:      channel,
		Subject:      subject,
		Content:      content,
		Data:         data,
		Status:       NotificationStatusPending,
		Metadata:     map[string]any{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// NewEmailNotification helper para crear notifications de email
func NewEmailNotification(
	userID uuid.UUID,
	subject string,
	content string,
	templateCode string,
	nType NotificationType,
	data map[string]any,
) *Notification {
	return NewNotification(
		userID,
		NotificationChannelEmail,
		content,
		nType,
		subject,
		templateCode,
		data,
	)
}

// MarkSent actualiza el estado a enviado con provider message ID
func (n *Notification) MarkSent(providerMessageID string) {
	n.Status = NotificationStatusSent
	n.ProviderMessageID = providerMessageID
	now := time.Now()
	n.SentAt = &now
	n.UpdatedAt = now
}

// MarkDelivered actualiza el estado a entregado (desde webhook)
func (n *Notification) MarkDelivered() {
	n.Status = NotificationStatusDelivered
	now := time.Now()
	n.DeliveredAt = &now
	n.UpdatedAt = now
}

// MarkOpened actualiza el estado a abierto (desde webhook)
func (n *Notification) MarkOpened() {
	n.Status = NotificationStatusDelivered
	now := time.Now()
	n.OpenedAt = &now
	n.UpdatedAt = now
}

// MarkFailed registra el error
func (n *Notification) MarkFailed(errMsg string) {
	n.Status = NotificationStatusFailed
	n.ErrorMessage = errMsg
	n.UpdatedAt = time.Now()
}

// MarkBounced marca como bounce (desde webhook)
func (n *Notification) MarkBounced() {
	n.Status = NotificationStatusBounced
	n.UpdatedAt = time.Now()
}

// IsRetryable indica si puede reintentarse (basado en status)
func (n *Notification) IsRetryable() bool {
	return n.Status == NotificationStatusPending || n.Status == NotificationStatusFailed
}

// IdempotencyKey devuelve una clave estable para deduplicación
func (n *Notification) IdempotencyKey() string {
	return string(n.Type) + ":" + n.UserID.String()
}
