// Consumer de eventos de conversaciones.
// Consume eventos "{events}:search.conversation.saved" de Dragonfly Streams
// y persiste las conversaciones en PostgreSQL para usuarios autenticados.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/shared/conversation"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Conversation Event Consumer — consume eventos y persiste en PG
// =============================================================================

// ConversationConsumer consumes conversation_saved events from Dragonfly Streams
// and persists them to PostgreSQL for authenticated users.
type ConversationConsumer struct {
	rdb        *redis.Client
	pgStore    *conversation.PgConversationStore
	group      string
	consumer   string
	streamName string
	running    atomic.Bool // true while the main consume loop is alive
}

// =============================================================================
// Constructor
// =============================================================================

// NewConversationConsumer creates a new conversation event consumer.
// Consumer names include Unix milliseconds for uniqueness across restarts.
func NewConversationConsumer(rdb *redis.Client, pgStore *conversation.PgConversationStore) *ConversationConsumer {
	return &ConversationConsumer{
		rdb:        rdb,
		pgStore:    pgStore,
		group:      "search-service",
		consumer:   fmt.Sprintf("search-worker-%d", time.Now().UnixMilli()),
		streamName: eventbus.StreamName("search.conversation.saved"),
	}
}

// =============================================================================
// Lifecycle
// =============================================================================

// Start begins consuming events from the conversation stream.
// Creates the consumer group (idempotent), launches the consume loop,
// and starts the orphan rescue goroutine.
func (c *ConversationConsumer) Start(ctx context.Context) error {
	// Ensure consumer group exists (idempotent)
	if err := eventbus.EnsureConsumerGroup(ctx, c.rdb, c.streamName, c.group); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	c.running.Store(true)
	go func() {
		defer c.running.Store(false)
		c.consume(ctx)
	}()
	go c.rescueOrphans(ctx)

	slog.InfoContext(ctx, "conversation consumer started",
		"group", c.group,
		"consumer", c.consumer,
		"stream", c.streamName,
	)
	return nil
}

// IsRunning reports whether the main consume goroutine is alive.
// Used by /ready health checks.
func (c *ConversationConsumer) IsRunning() bool { return c.running.Load() }

// Name returns a human-readable identifier for health check reporting.
func (c *ConversationConsumer) Name() string { return "search-conversation-consumer" }

// =============================================================================
// Worker loop
// =============================================================================

// consume reads new messages from the stream using XREADGROUP.
// Uses Block=5s for passive waiting (zero CPU while idle).
// Count=10 limits the batch size per read.
func (c *ConversationConsumer) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  []string{c.streamName, ">"}, // ">" = only new, unassigned messages
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		// redis.Nil means the block timed out with no messages — normal idle state
		if err == redis.Nil {
			continue
		}
		if err != nil {
			slog.ErrorContext(ctx, "xreadgroup error", "error", err)
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

// processMessage handles a single stream message.
// Malformed messages (missing/invalid event_type) are ACKed to avoid
// permanently blocking the PEL. Valid messages that fail processing
// are NOT ACKed — they stay in the PEL for retry via rescueOrphans.
func (c *ConversationConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	// Get event type directly from flat payload
	eventTypeVal, ok := msg.Values["event_type"]
	if !ok {
		slog.ErrorContext(ctx, "missing event_type in message", "msg_id", msg.ID)
		if err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID).Err(); err != nil {
			if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "nil") {
				slog.ErrorContext(ctx, "xack error", "error", err, "msg_id", msg.ID)
			}
		}
		return
	}
	eventType, ok := eventTypeVal.(string)
	if !ok {
		slog.ErrorContext(ctx, "invalid event_type type", "msg_id", msg.ID, "value", eventTypeVal)
		if err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID).Err(); err != nil {
			if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "nil") {
				slog.ErrorContext(ctx, "xack error", "error", err, "msg_id", msg.ID)
			}
		}
		return
	}

	// Handle based on type
	var handleErr error
	switch eventType {
	case string(eventbus.ConversationSaved):
		handleErr = c.handleConversationSaved(ctx, msg.Values)
	default:
		slog.WarnContext(ctx, "unknown event type in conversation consumer", "type", eventType, "msg_id", msg.ID)
		if err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID).Err(); err != nil {
			if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "nil") {
				slog.ErrorContext(ctx, "xack error", "error", err, "msg_id", msg.ID)
			}
		}
		return
	}

	// On failure, leave message in PEL for retry
	if handleErr != nil {
		slog.ErrorContext(ctx, "failed to handle conversation saved event",
			"error", handleErr,
			"msg_id", msg.ID,
		)
		return
	}

	// ACK only if handling succeeded
	if count, err := c.rdb.XAck(ctx, c.streamName, c.group, msg.ID).Result(); err != nil {
		if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "nil") {
			slog.ErrorContext(ctx, "xack error", "error", err, "msg_id", msg.ID)
		}
	} else {
		slog.DebugContext(ctx, "xack success", "count", count, "msg_id", msg.ID)
	}
}

// =============================================================================
// Manejador de eventos: conversation_saved
// =============================================================================

// handleConversationSaved extracts fields from the flat event payload and persists
// the conversation to PostgreSQL via the PgConversationStore.
//
// Flat payload: all fields are at the top level of the map (matching the pattern
// used by register/usecase.go). This avoids go-redis marshal issues with nested
// map[string]interface{} values.
func (c *ConversationConsumer) handleConversationSaved(ctx context.Context, values map[string]any) error {
	// Flat payload — all fields are directly in values (no nested "payload" key)
	payload := values

	// Extract conversation_id (string UUID)
	conversationID, ok := payload["conversation_id"].(string)
	if !ok || conversationID == "" {
		return fmt.Errorf("missing or invalid conversation_id in payload")
	}

	// Extract user_id (string UUID)
	userID, ok := payload["user_id"].(string)
	if !ok || userID == "" {
		return fmt.Errorf("missing or invalid user_id in payload")
	}

	// Extract messages JSON
	messagesRaw, ok := payload["messages"].(string)
	if !ok {
		return fmt.Errorf("missing messages in payload")
	}

	// Extract turn_count (int64 from go-redis XAdd → int).
	// go-redis stores int64 as int64 directly; JSON would use float64.
	// Following the pattern in EventFromMap for timestamp parsing.
	turnCount, err := parseIntField(payload, "turn_count")
	if err != nil {
		return fmt.Errorf("turn_count: %w", err)
	}

	// Extract max_turns (int64 from go-redis XAdd → int)
	maxTurns, err := parseIntField(payload, "max_turns")
	if err != nil {
		return fmt.Errorf("max_turns: %w", err)
	}

	// Extract created_at (RFC3339 string → time.Time)
	caVal, ok := payload["created_at"].(string)
	if !ok {
		return fmt.Errorf("missing created_at in payload")
	}
	createdAt, err := time.Parse(time.RFC3339, caVal)
	if err != nil {
		return fmt.Errorf("parse created_at %q: %w", caVal, err)
	}

	// Build ConversationState from payload fields
	conv := &domain.ConversationState{
		ID:        conversationID,
		UserID:    userID,
		TurnCount: int(turnCount),
		MaxTurns:  int(maxTurns),
		CreatedAt: createdAt,
	}

	// Unmarshal messages JSON
	if err := json.Unmarshal([]byte(messagesRaw), &conv.Messages); err != nil {
		return fmt.Errorf("unmarshal messages: %w", err)
	}

	// Optional: intent JSONB
	if intentStr, ok := payload["intent"].(string); ok && intentStr != "" {
		conv.Intent = &domain.TravelIntent{}
		if err := json.Unmarshal([]byte(intentStr), conv.Intent); err != nil {
			return fmt.Errorf("unmarshal intent: %w", err)
		}
	}

	// Optional: results JSONB
	if resultsStr, ok := payload["results"].(string); ok && resultsStr != "" {
		conv.Results = json.RawMessage(resultsStr)
	}

	// Persist to PostgreSQL
	c.pgStore.SaveConversationHistory(ctx, conv)

	return nil
}

// =============================================================================
// Rescate de mensajes huérfanos
// =============================================================================

// rescueOrphans periodically reclaims messages that have been idle in the PEL
// for more than 5 minutes (e.g. because the original consumer crashed before ACKing).
// Runs every 30 seconds.
func (c *ConversationConsumer) rescueOrphans(ctx context.Context) {
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
			slog.ErrorContext(ctx, "rescue orphans error", "error", err)
			continue
		}

		for _, msg := range messages {
			slog.InfoContext(ctx, "reclaiming orphan conversation message", "msg_id", msg.ID)
			c.processMessage(ctx, msg)
		}
	}
}

// =============================================================================
// Helpers
// =============================================================================

// parseIntField extracts an integer field from the event payload.
// go-redis XReadGroup returns ALL stream values as strings, regardless
// of how they were stored via XAdd. We handle string, float64 (JSON),
// int64, and int for maximum compatibility.
func parseIntField(payload map[string]any, key string) (int, error) {
	val, ok := payload[key]
	if !ok {
		return 0, fmt.Errorf("missing field %q in payload", key)
	}
	switch v := val.(type) {
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("field %q is not a valid integer: %q", key, v)
		}
		return n, nil
	case float64:
		return int(v), nil
	case int64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("field %q is not a number: %T", key, val)
	}
}
