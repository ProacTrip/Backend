// Consumer de eventos de notificaciones.
// Consume eventos de Dragonfly Streams y procesaenvíos de emails.
package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/notification/features/send_verification_email"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Notification Event Consumer - Consume eventos y envía emails
// =============================================================================

type NotificationConsumer struct {
	rdb        *redis.Client
	usecase    *send_verification_email.UseCase
	group      string
	consumer   string
	streamName string
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

// Start comienza a consumir eventos
func (c *NotificationConsumer) Start(ctx context.Context) error {
	// Ensure consumer group exists (idempotent)
	if err := eventbus.EnsureConsumerGroup(ctx, c.rdb, c.streamName, c.group); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	go c.consume(ctx)
	go c.rescueOrphans(ctx)

	slog.Info("notification consumer started", "group", c.group, "consumer", c.consumer)
	return nil
}

// =============================================================================
// Worker loop
// =============================================================================

func (c *NotificationConsumer) consume(ctx context.Context) {
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

		for _, stream := range messages {
			for _, msg := range stream.Messages {
				c.processMessage(ctx, msg)
			}
		}
	}
}

// =============================================================================
// Procesamiento de mensajes
// =============================================================================

func (c *NotificationConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	// Get event type directly from flat payload
	eventTypeVal, ok := msg.Values["event_type"]
	if !ok {
		slog.Error("missing event_type in message", "msg_id", msg.ID)
		if err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID).Err(); err != nil {
			// Only log if it's a real error (network, timeout, etc)
			if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "nil") {
				slog.Error("xack error", "error", err, "msg_id", msg.ID)
			}
		}
		return
	}
	eventType, ok := eventTypeVal.(string)
	if !ok {
		slog.Error("invalid event_type type", "msg_id", msg.ID, "value", eventTypeVal)
		if err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID).Err(); err != nil {
			// Only log if it's a real error (network, timeout, etc)
			if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "nil") {
				slog.Error("xack error", "error", err, "msg_id", msg.ID)
			}
		}
		return
	}

	// Handle based on type
	var handleErr error
	switch eventType {
	case string(eventbus.UserRegistered):
		handleErr = c.handleUserRegistered(ctx, msg.Values)
	default:
		slog.Warn("unknown event type", "type", eventType, "msg_id", msg.ID)
		if err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID).Err(); err != nil {
			// Only log if it's a real error (network, timeout, etc)
			if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "nil") {
				slog.Error("xack error", "error", err, "msg_id", msg.ID)
			}
		}
		return
	}

	if handleErr != nil {
		slog.Error("failed to handle event", "error", handleErr, "msg_id", msg.ID)
		// Don't ACK — leave in PEL for retry
		return
	}

	// ACK only if handling succeeded
	if count, err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID).Result(); err != nil {
		// Only log if it's a real error (network, timeout, etc)
		// Ignore if it's just "no such message" or "already acked"
		if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "nil") {
			slog.Error("xack error", "error", err, "msg_id", msg.ID)
		}
	} else {
		slog.Debug("xack success", "count", count, "msg_id", msg.ID)
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

	// Get verification token from payload (generated by auth module)
	verificationToken, _ := payload["verification_token"].(string)
	if verificationToken == "" {
		// Token no está en el evento - el usuario deberá solicitar reenvío
		// o el token se genera en el momento de verificar
		slog.Warn("verification token not in event, email will require user action", "user_id", userID)
		verificationToken = "pending" // Placeholder
	}

	// Send verification email
	if err := c.usecase.Execute(ctx, userID, email, verificationToken); err != nil {
		slog.Error("failed to send verification email", "error", err, "user_id", userID)
		// Don't ack - leave in PEL for retry
		return err
	}

	slog.Info("verification email queued", "user_id", userID, "email", email)
	return nil
}

// rescueOrphans re-clama mensajes huérfanos cada 30 segundos
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
