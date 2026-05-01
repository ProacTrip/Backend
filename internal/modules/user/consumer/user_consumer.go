// Consumer de eventos de usuario.
// Consume eventos de Dragonfly Streams y crea/actualiza perfiles.
package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upsert_profile"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// User Event Consumer - Consume eventos de Dragonfly Streams
// Patrón Upsert para manejar la condición de carrera verify-email vs profile creation
// =============================================================================

type UserEventConsumer struct {
	rdb        *redis.Client
	repo       domain.UserRepository
	uc         *upsert_profile.UseCase
	group      string
	consumer   string
	streamName string
	running    atomic.Bool // true while the main consume loop is alive
}

// =============================================================================
// Constructor
// =============================================================================

func NewUserEventConsumer(rdb *redis.Client, repo domain.UserRepository) *UserEventConsumer {
	return &UserEventConsumer{
		rdb:        rdb,
		repo:       repo,
		uc:         upsert_profile.NewUseCase(repo),
		group:      "user-service",
		consumer:   fmt.Sprintf("user-worker-%d", time.Now().UnixMilli()),
		streamName: eventbus.StreamName("auth.user.registered"),
	}
}

// =============================================================================
// Inicio del consumer
// =============================================================================

// Start Begins consuming events from the stream
func (c *UserEventConsumer) Start(ctx context.Context) error {
	// Ensure consumer group exists (idempotent)
	if err := eventbus.EnsureConsumerGroup(ctx, c.rdb, c.streamName, c.group); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	// Start worker loop
	c.running.Store(true)
	go func() {
		defer c.running.Store(false)
		c.consume(ctx)
	}()

	// Start orphan rescue worker (XAUTOCLAIM)
	go c.rescueOrphans(ctx)

	slog.Info("user event consumer started", "group", c.group, "consumer", c.consumer)
	return nil
}

// IsRunning reports whether the main consume goroutine is alive.
// Used by /ready health checks.
func (c *UserEventConsumer) IsRunning() bool { return c.running.Load() }

// Name returns a human-readable identifier for health check reporting.
func (c *UserEventConsumer) Name() string { return "user-consumer" }

// =============================================================================
// Worker loop
// =============================================================================

func (c *UserEventConsumer) consume(ctx context.Context) {
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

func (c *UserEventConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	// Parse event from message values
	event, err := eventbus.EventFromMap(msg.Values)
	if err != nil {
		slog.Error("parse event error", "error", err, "msg_id", msg.ID)
		// Ack malformed messages to avoid stuck in PEL
		_ = c.rdb.XAck(ctx, c.streamName, c.group, msg.ID)
		return
	}

	// Handle event based on type - using Go 1.26+ errors.AsType pattern
	switch event.EventType {
	case eventbus.UserRegistered:
		c.handleUserRegistered(ctx, event)
	default:
		slog.Warn("unknown event type", "type", event.EventType)
	}

	// Always ack after processing
	if err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID); err != nil {
		slog.Error("xack error", "error", err, "msg_id", msg.ID)
	}
}

func (c *UserEventConsumer) handleUserRegistered(ctx context.Context, event *eventbus.Event) {
	// Extract user data from payload
	payload := event.Payload

	userIDStr, ok := payload["user_id"].(string)
	if !ok {
		slog.Error("missing user_id in payload")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		slog.Error("invalid user_id format", "user_id", userIDStr)
		return
	}

	// Create profile using Upsert use case (inyectado en constructor, no crear en cada mensaje)
	// El perfil se crea basado en user_id - el email viene del dominio Auth
	if err := c.uc.Execute(ctx, userID); err != nil {
		slog.Error("upsert profile failed", "error", err, "user_id", userID)
		// Don't ack - leave in PEL for retry
		return
	}

	slog.Info("user profile created/updated", "user_id", userID)
}

// rescueOrphans runs XAUTOCLAIM periodically to reclaim messages from dead workers
func (c *UserEventConsumer) rescueOrphans(ctx context.Context) {
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
