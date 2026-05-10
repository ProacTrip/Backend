// Domain: Entidades y tipos de dominio para notificaciones.
// Define la estructura de una notificación y sus estados.
package domain

import (
	"errors"
	"fmt"
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

// NotificationStatus representa el estado de entrega de una notificación.
// Los valores están alineados con el CHECK constraint de la base de datos.
type NotificationStatus string

// Estados válidos para una notificación según la máquina de estados del dominio.
const (
	NotificationStatusPending   NotificationStatus = "pending"   // Estado inicial.
	NotificationStatusSent      NotificationStatus = "sent"      // Enviada al proveedor.
	NotificationStatusDelivered NotificationStatus = "delivered" // Confirmada entregada por webhook.
	NotificationStatusOpened    NotificationStatus = "opened"    // Abierta por el destinatario.
	NotificationStatusFailed    NotificationStatus = "failed"    // Falló el envío.
	NotificationStatusBounced   NotificationStatus = "bounced"   // Rebotada por el proveedor.
)

// =============================================================================
// Máquina de estados — transiciones válidas
// =============================================================================

// validTransitions define las transiciones de estado permitidas para NotificationStatus.
var validTransitions = map[NotificationStatus][]NotificationStatus{
	NotificationStatusPending:   {NotificationStatusSent, NotificationStatusFailed},
	NotificationStatusSent:      {NotificationStatusDelivered, NotificationStatusFailed, NotificationStatusBounced},
	NotificationStatusDelivered: {NotificationStatusOpened, NotificationStatusBounced},
	NotificationStatusOpened:    {},
	NotificationStatusFailed:    {NotificationStatusPending},
	NotificationStatusBounced:   {},
}

// ErrInvalidStateTransition se retorna cuando se intenta una transición de estado
// que no está definida en validTransitions.
var ErrInvalidStateTransition = errors.New("NOTIF_INVALID_TRANSITION: transición de estado inválida")

// Transition valida que la transición al estado target sea permitida según la
// máquina de estados definida en validTransitions.
func (n *Notification) Transition(target NotificationStatus) error {
	allowed, ok := validTransitions[n.Status]
	if !ok {
		return ErrInvalidStateTransition
	}
	for _, s := range allowed {
		if s == target {
			return nil
		}
	}
	return ErrInvalidStateTransition
}

// Notification representa una notificación completa alineada con la migración 001_notifications.
// Contiene todos los campos del registro incluyendo tracking de entrega y reintentos.
type Notification struct {
	ID                uuid.UUID           `json:"id"`
	UserID            uuid.UUID           `json:"user_id"`
	TemplateCode      string              `json:"template_code,omitempty"` // ID del template de Resend.
	Type              NotificationType    `json:"type"`                    // transactional/marketing/system
	Channel           NotificationChannel `json:"channel"`                 // email/sms/websocket (NOT NULL)
	Subject           string              `json:"subject,omitempty"`
	Content           string              `json:"content"` // NOT NULL
	Data              map[string]any      `json:"data,omitempty"`
	Status            NotificationStatus  `json:"status"` // pending/sent/delivered/opened/failed/bounced
	SentAt            *time.Time          `json:"sent_at,omitzero"`
	DeliveredAt       *time.Time          `json:"delivered_at,omitzero"`
	OpenedAt          *time.Time          `json:"opened_at,omitzero"`
	ErrorMessage      string              `json:"error_message,omitempty"`
	ProviderMessageID string              `json:"provider_message_id,omitempty"` // ID del mensaje en Resend.
	RetryCount        int                 `json:"retry_count,omitempty"`         // Contador de reintentos (solo en memoria).
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
) (*Notification, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generar UUID de notificación: %w", err)
	}
	now := time.Now()
	return &Notification{
		ID:           id,
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
	}, nil
}

// NewEmailNotification helper para crear notificaciones de email.
func NewEmailNotification(
	userID uuid.UUID,
	subject string,
	content string,
	nType NotificationType,
	templateCode string,
	data map[string]any,
) (*Notification, error) {
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

// MarkSent actualiza el estado a enviado con el provider message ID de Resend.
// Si la notificación venía de un reintento (estado failed → pending → sent),
// el RetryCount se mantiene para trazabilidad de cuántos intentos tomó.
func (n *Notification) MarkSent(providerMessageID string) error {
	if err := n.Transition(NotificationStatusSent); err != nil {
		return err
	}
	n.Status = NotificationStatusSent
	n.ProviderMessageID = providerMessageID
	now := time.Now()
	n.SentAt = &now
	n.UpdatedAt = now
	return nil
}

// MarkDelivered actualiza el estado a entregado (desde webhook).
func (n *Notification) MarkDelivered() error {
	if err := n.Transition(NotificationStatusDelivered); err != nil {
		return err
	}
	n.Status = NotificationStatusDelivered
	now := time.Now()
	n.DeliveredAt = &now
	n.UpdatedAt = now
	return nil
}

// MarkOpened actualiza el estado a abierto (desde webhook).
func (n *Notification) MarkOpened() error {
	if err := n.Transition(NotificationStatusOpened); err != nil {
		return err
	}
	n.Status = NotificationStatusOpened
	now := time.Now()
	n.OpenedAt = &now
	n.UpdatedAt = now
	return nil
}

// MarkFailed registra el error de envío e incrementa el contador de reintentos.
func (n *Notification) MarkFailed(errMsg string) error {
	if err := n.Transition(NotificationStatusFailed); err != nil {
		return err
	}
	n.Status = NotificationStatusFailed
	n.ErrorMessage = errMsg
	n.ProviderMessageID = "" // Limpiar ID residual de intento previo.
	n.RetryCount++
	n.UpdatedAt = time.Now()
	return nil
}

// MarkBounced marca como rebotado (desde webhook).
func (n *Notification) MarkBounced() error {
	if err := n.Transition(NotificationStatusBounced); err != nil {
		return err
	}
	n.Status = NotificationStatusBounced
	n.UpdatedAt = time.Now()
	return nil
}

// IsRetryable indica si puede reintentarse (basado en status)
func (n *Notification) IsRetryable() bool {
	return n.Status == NotificationStatusPending || n.Status == NotificationStatusFailed
}

// CanRetry verifica que la notificación esté en un estado reintentable
// y que no haya superado el máximo de intentos configurado.
// Retorna false si maxAttempts <= 0 (política sin reintentos).
func (n *Notification) CanRetry(maxAttempts int) bool {
	if maxAttempts <= 0 {
		return false
	}
	return n.IsRetryable() && n.RetryCount < maxAttempts
}


