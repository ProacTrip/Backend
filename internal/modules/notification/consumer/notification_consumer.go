// Consumer de eventos de notificaciones.
// Consume eventos de Dragonfly Streams y procesa envíos de emails.
// Usa backoff exponencial cuando el handler falla para evitar
// reintentos en tight-loop y proteger la base de datos.
package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/notification/features/send_verification_email"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Consumer de Eventos de Notificaciones
// Consume eventos del stream de Dragonfly y despacha envíos de email.
// =============================================================================

// NotificationConsumer consume eventos del stream de notificaciones
// y los despacha al caso de uso correspondiente.
type NotificationConsumer struct {
	rdb        *redis.Client
	usecase    *send_verification_email.UseCase
	group      string
	consumer   string
	streamName string
	running    atomic.Bool // true mientras el loop principal de consumo está vivo
}

// =============================================================================
// Constructor
// =============================================================================

func NewNotificationConsumer(rdb *redis.Client, uc *send_verification_email.UseCase) *NotificationConsumer {
	return &NotificationConsumer{
		rdb:        rdb,
		usecase:    uc,
		group:      "notification-service",
		consumer:   fmt.Sprintf("notification-worker-%d", time.Now().UnixMilli()),
		streamName: eventbus.StreamName("auth.user.registered"),
	}
}

// Start comienza a consumir eventos del stream.
func (c *NotificationConsumer) Start(ctx context.Context) error {
	// Asegurar que el consumer group existe (idempotente)
	if err := eventbus.EnsureConsumerGroup(ctx, c.rdb, c.streamName, c.group); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	c.running.Store(true)
	go func() {
		defer c.running.Store(false)
		c.consume(ctx)
	}()
	go c.rescueOrphans(ctx)

	slog.Info("notification consumer started", "group", c.group, "consumer", c.consumer)
	return nil
}

// IsRunning reporta si el goroutine principal de consumo está vivo.
// Usado por health checks de /ready.
func (c *NotificationConsumer) IsRunning() bool { return c.running.Load() }

// Name retorna un identificador legible para reportes de health check.
func (c *NotificationConsumer) Name() string { return "notification-consumer" }

// =============================================================================
// Worker loop con backoff exponencial
// =============================================================================

func (c *NotificationConsumer) consume(ctx context.Context) {
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
	)
	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  []string{c.streamName, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			slog.Error("xreadgroup error", "error", err)
			continue
		}

		// Resetear backoff cuando XReadGroup funciona — el stream está sano.
		backoff = initialBackoff

		// Procesar mensajes del lote. Si alguno falla, hacer backoff
		// antes del siguiente XReadGroup para evitar tight-loop de reintentos.
		hadFailure := false
		for _, stream := range messages {
			for _, msg := range stream.Messages {
				if err := c.processMessage(ctx, msg); err != nil {
					hadFailure = true
					// No detenemos el procesamiento del lote — intentamos
					// procesar los demás mensajes aunque uno falle.
				}
			}
		}

		if hadFailure {
			slog.Warn("algunos mensajes del lote fallaron, esperando antes de reintentar",
				slog.Duration("backoff", backoff),
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// =============================================================================
// Procesamiento de mensajes
// =============================================================================

// processMessage procesa un mensaje individual del stream.
// Retorna error si el handler falla (el mensaje queda en PEL para reintento).
// Retorna nil si el mensaje fue procesado exitosamente o si fue descartado
// (ACK) por ser inválido (ej. sin event_type).
func (c *NotificationConsumer) processMessage(ctx context.Context, msg redis.XMessage) error {
	// Obtener event_type del payload plano
	eventTypeVal, ok := msg.Values["event_type"]
	if !ok {
		slog.Error("missing event_type in message", "msg_id", msg.ID)
		c.ackOrWarn(ctx, msg.ID)
		return nil // Descartado — ya se hizo ACK, no reintentar.
	}
	eventType, ok := eventTypeVal.(string)
	if !ok {
		slog.Error("invalid event_type type", "msg_id", msg.ID, "value", eventTypeVal)
		c.ackOrWarn(ctx, msg.ID)
		return nil // Descartado — ya se hizo ACK, no reintentar.
	}

	// Despachar según tipo de evento
	var handleErr error
	switch eventType {
	case string(eventbus.UserRegistered):
		handleErr = c.handleUserRegistered(ctx, msg.Values)
	default:
		slog.Warn("unknown event type", "type", eventType, "msg_id", msg.ID)
		c.ackOrWarn(ctx, msg.ID)
		return nil // Descartado — tipo desconocido, ACK y seguir.
	}

	if handleErr != nil {
		slog.Error("failed to handle event", "error", handleErr, "msg_id", msg.ID)
		// No hacer ACK — dejar en PEL para reintento con backoff.
		return handleErr
	}

	// ACK solo si el handler fue exitoso.
	c.ackOrWarn(ctx, msg.ID)
	return nil
}

// ackOrWarn confirma el mensaje en el stream. Si XAck falla (ej. error de red,
// mensaje ya confirmado), loguea una advertencia pero nunca crashea el consumer.
func (c *NotificationConsumer) ackOrWarn(ctx context.Context, msgID string) {
	count, err := c.rdb.XAck(ctx, c.streamName, c.group, msgID).Result()
	if err != nil {
		// Error de red, timeout, o mensaje ya no está en PEL.
		// Loguear y seguir — nunca crashear el consumer por un ACK.
		slog.Warn("xack warning (no bloqueante)",
			slog.String("error", err.Error()),
			slog.String("msg_id", msgID),
		)
		return
	}
	if count == 0 {
		slog.Debug("xack: mensaje ya estaba confirmado", "msg_id", msgID)
	} else {
		slog.Info("xack success", "count", count, "msg_id", msgID)
	}
}

func (c *NotificationConsumer) handleUserRegistered(ctx context.Context, payload map[string]interface{}) error {
	userIDStr, ok := payload["user_id"].(string)
	if !ok {
		slog.Error("missing user_id in payload")
		return fmt.Errorf("missing user_id")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		slog.Error("invalid user_id format", "user_id", userIDStr)
		return fmt.Errorf("invalid user_id: %w", err)
	}

	email, ok := payload["email"].(string)
	if !ok {
		slog.Error("missing email in payload")
		return fmt.Errorf("missing email")
	}

	// Si el usuario se registró vía OAuth (Google, GitHub, etc.), el email ya fue
	// verificado por el proveedor — no hay que enviar email de verificación.
	provider, hasProvider := payload["provider"].(string)
	if hasProvider && provider != "" {
		slog.InfoContext(ctx, "skipping verification email for OAuth user",
			"user_id", userID, "provider", provider,
		)
		return nil
	}

	// Verificación secundaria: email_verified flag explícito en el payload.
	if verified, ok := payload["email_verified"].(bool); ok && verified {
		slog.InfoContext(ctx, "skipping verification email (email_verified=true in payload)",
			"user_id", userID,
		)
		return nil
	}

	// Obtener token de verificación del payload (generado por el módulo auth)
	verificationToken, _ := payload["verification_token"].(string)
	if verificationToken == "" {
		// Token no está en el evento — el usuario deberá solicitar reenvío
		// o el token se genera en el momento de verificar
		slog.Warn("verification token not in event, email will require user action", "user_id", userID)
		verificationToken = "pending" // Placeholder
	}

	// Extraer first_name del payload (opcional)
	firstName, _ := payload["first_name"].(string)

	// Enviar email de verificación
	cmd := send_verification_email.Command{
		UserID:            userID,
		Email:             email,
		VerificationToken: verificationToken,
		FirstName:         firstName,
	}
	if _, err := c.usecase.Execute(ctx, cmd); err != nil {
		slog.Error("failed to send verification email", "error", err, "user_id", userID)
		// No hacer ACK — dejar en PEL para reintento con backoff.
		return err
	}

	slog.Info("verification email sent", "user_id", userID, "email", email)
	return nil
}

// rescueOrphans reclama mensajes huérfanos cada 30 segundos.
// Usa XAUTOCLAIM para reasignar mensajes que quedaron en PEL
// porque el worker original crasheó antes de hacer ACK.
func (c *NotificationConsumer) rescueOrphans(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		messages, err := eventbus.RescueOrphanedMessages(ctx, c.rdb, c.streamName, c.group, 5*time.Minute)
		if err != nil {
			slog.Error("rescue orphans error", "error", err)
			continue
		}

		for _, msg := range messages {
			slog.Info("reclaiming orphan message", "msg_id", msg.ID)
			c.processMessage(ctx, msg)
		}
	}
}
