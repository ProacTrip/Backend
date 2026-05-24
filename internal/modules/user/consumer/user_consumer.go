// Consumer de eventos de usuario.
// Consume eventos de Dragonfly Streams y crea/actualiza perfiles.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upsert_profile"
	sharedEnvironment "github.com/ProacTrip/Backend/internal/shared/environment"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// User Event Consumer - Consume eventos de Dragonfly Streams
// Patrón Upsert para manejar la condición de carrera verify-email vs profile creation
// =============================================================================

type UserEventConsumer struct {
	rdb        *redis.Client
	repo       domain.ProfileRepository
	uc         *upsert_profile.UseCase
	group      string
	consumer   string
	streamName string
	running    atomic.Bool // true while the main consume loop is alive
}

// =============================================================================
// Constructor
// =============================================================================

func NewUserEventConsumer(
	rdb *redis.Client,
	repo domain.ProfileRepository,
	travelRepo domain.TravelPrefsRepository,
	medicalRepo domain.MedicalProfileRepository,
) *UserEventConsumer {
	return &UserEventConsumer{
		rdb:  rdb,
		repo: repo,
		uc: upsert_profile.NewUseCaseComplete(
			repo, travelRepo, medicalRepo, rdb,
		),
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

		// Reset backoff when XReadGroup succeeds — stream is healthy.
		backoff = initialBackoff

		hadFailure := false
		for _, stream := range messages {
			for _, msg := range stream.Messages {
				if err := c.processMessage(ctx, msg); err != nil {
					hadFailure = true
					// Continue processing remaining messages in the batch.
				}
			}
		}

		if hadFailure {
			slog.Warn("some messages failed, backing off before retry",
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

func (c *UserEventConsumer) processMessage(ctx context.Context, msg redis.XMessage) error {
	// Parse event from message values
	event, err := eventbus.EventFromMap(msg.Values)
	if err != nil {
		slog.Error("parse event error", "error", err, "msg_id", msg.ID)
		// Ack malformed messages to avoid stuck in PEL
		_ = c.rdb.XAck(ctx, c.streamName, c.group, msg.ID)
		return nil
	}

	// Handle event based on type
	switch event.EventType {
	case eventbus.UserRegistered:
		if err := c.handleUserRegistered(ctx, event); err != nil {
			return err // Don't ACK — leave in PEL for retry
		}
	default:
		slog.Warn("unknown event type", "type", event.EventType)
	}

	// ACK only after successful processing
	if err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID); err != nil {
		slog.Error("xack error", "error", err, "msg_id", msg.ID)
	}
	return nil
}

func (c *UserEventConsumer) handleUserRegistered(ctx context.Context, event *eventbus.Event) error {
	// Extract user data from payload
	payload := event.Payload

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

	// Extract email from the registration event (denormalized into user_profiles)
	email, _ := payload["email"].(string)

	// Extract optional environment fields from the event (may be absent in legacy events)
	envPrefs := extractEnvPrefs(payload)

	// Extract optional first_name from the registration event
	firstName, _ := payload["first_name"].(string)

	// Extract optional avatar_url from the registration event (e.g., Google OAuth picture)
	avatarURL, _ := payload["avatar_url"].(string)

	// Fallback: si el evento no incluye environment preferences, intentar leer
	// del caché env:{ip} en DragonflyDB (escrito por el módulo environment).
	if !envPrefs.HasAny() {
		if clientIP, ok := payload["client_ip"].(string); ok && clientIP != "" {
			envPrefs = c.resolveEnvPrefsFromCache(ctx, clientIP)
		}
	}

	// Create profile using Upsert use case (inyectado en constructor, no crear en cada mensaje)
	if err := c.uc.Execute(ctx, userID, email, firstName, avatarURL, envPrefs); err != nil {
		slog.Error("upsert profile failed", "error", err, "user_id", userID)
		return fmt.Errorf("upsert profile: %w", err)
	}

	slog.Info("user profile created/updated", "user_id", userID, "email", email)
	return nil
}

// extractEnvPrefs extracts optional environment preference fields from the event payload.
// Returns zero-value EnvPrefs if none are present (old events).
func extractEnvPrefs(payload map[string]interface{}) domain.EnvPrefs {
	var prefs domain.EnvPrefs

	if v, ok := payload["language_code"].(string); ok && v != "" {
		prefs.LanguageCode = v
	}
	if v, ok := payload["currency_code"].(string); ok && v != "" {
		prefs.CurrencyCode = v
	}
	if v, ok := payload["country_code"].(string); ok && v != "" {
		prefs.CountryCode = v
	}
	if v, ok := payload["timezone_name"].(string); ok && v != "" {
		prefs.TimezoneName = v
	}

	return prefs
}

// resolveEnvPrefsFromCache intenta leer las preferencias de entorno desde el
// caché env:{ip} en DragonflyDB cuando el evento de registro no las incluye.
// Retorna EnvPrefs con los campos poblados si el caché existe y es válido;
// EnvPrefs zero-value en caso de cache miss o error de deserialización.
func (c *UserEventConsumer) resolveEnvPrefsFromCache(ctx context.Context, clientIP string) domain.EnvPrefs {
	key := sharedEnvironment.CacheKey(clientIP)
	raw, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			slog.Warn("env cache lookup failed for user consumer fallback",
				slog.String("ip", clientIP),
				slog.String("error", err.Error()),
			)
		}
		return domain.EnvPrefs{}
	}
	if raw == "" {
		return domain.EnvPrefs{}
	}

	var entry sharedEnvironment.CacheEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		slog.Warn("env cache unmarshal failed for user consumer fallback",
			slog.String("ip", clientIP),
			slog.String("error", err.Error()),
		)
		return domain.EnvPrefs{}
	}

	return domain.EnvPrefs{
		LanguageCode: entry.Location.Language,
		CurrencyCode: entry.Location.Currency,
		CountryCode:  entry.Location.CountryCode,
		TimezoneName: entry.Location.Timezone,
	}
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
			if err := c.processMessage(ctx, msg); err != nil {
				slog.Error("orphan message processing failed", "error", err, "msg_id", msg.ID)
			}
		}
	}
}
