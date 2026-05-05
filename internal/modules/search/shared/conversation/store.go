// conversation — conversation state persistence for AI search multi-turn sessions.
//
// DESIGN: Two-tier storage with event-driven PG persistence:
//   1. Dragonfly Hash with HEXPIRE per field (10min TTL) — primary, low-latency store
//      for all users (auth + anonymous). Each field expires independently.
//   2. Dragonfly Streams event sourcing → async PostgreSQL writes via consumer.
//      SaveConversation publishes "{events}:search.conversation.saved" and returns
//      immediately. A separate ConversationConsumer reads from the stream and
//      persists to PostgreSQL. This decouples the hot-path from PG latency.
//      Anonymous users (UserID == "") do NOT trigger events.
//
// Key format: ai:conv:{conversationID}
// Hash fields: id, user_id, messages, intent, results, turn_count, max_turns, created_at, expires_at
//
// Anonymous users are Dragonfly-only — no PG writes, no events published.
package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Constants
// =============================================================================

// ConversationTTL is the TTL for conversation Hash fields in Dragonfly.
// Each field expires independently via HEXPIRE.
const ConversationTTL = 10 * time.Minute

// =============================================================================
// Errors
// =============================================================================

// ErrConversationStoreFailed is returned when a Dragonfly operation fails.
var ErrConversationStoreFailed = errors.New("CONVERSATION_STORE_FAILED: dragonfly operation failed")

// =============================================================================
// Key format
// =============================================================================

// conversationKey returns the Dragonfly Hash key for a conversation.
func conversationKey(id string) string {
	return fmt.Sprintf("ai:conv:%s", id)
}

// =============================================================================
// SaveConversation
// =============================================================================

// SaveConversation persists a ConversationState to Dragonfly.
// Uses Hash with HEXPIRE per field for independent TTLs.
// Auth users also trigger an async PG write via SaveConversationHistory.
//
// Returns ErrConversationStoreFailed on Dragonfly errors.
func SaveConversation(ctx context.Context, rdb *redis.Client, conv *domain.ConversationState) error {
	if conv == nil {
		return errors.New("cannot save nil conversation")
	}
	if conv.ID == "" {
		return errors.New("conversation ID must not be empty")
	}

	key := conversationKey(conv.ID)

	// Serialize complex fields to JSON
	messagesJSON, err := json.Marshal(conv.Messages)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	var intentJSON []byte
	if conv.Intent != nil {
		intentJSON, err = json.Marshal(conv.Intent)
		if err != nil {
			return fmt.Errorf("marshal intent: %w", err)
		}
	}

	var resultsJSON []byte
	if len(conv.Results) > 0 {
		resultsJSON = make([]byte, len(conv.Results))
		copy(resultsJSON, conv.Results)
	}

	ttlSeconds := int(ConversationTTL.Seconds())

	// Use pipeline for atomic Hash set + HEXPIRE per field
	pipe := rdb.Pipeline()

	// Set all fields
	pipe.HSet(ctx, key,
		"id", conv.ID,
		"user_id", conv.UserID,
		"messages", string(messagesJSON),
		"turn_count", conv.TurnCount,
		"max_turns", conv.MaxTurns,
		"created_at", conv.CreatedAt.Format(time.RFC3339),
		"expires_at", conv.ExpiresAt.Format(time.RFC3339),
	)

	if len(intentJSON) > 0 {
		pipe.HSet(ctx, key, "intent", string(intentJSON))
	}
	if len(resultsJSON) > 0 {
		pipe.HSet(ctx, key, "results", string(resultsJSON))
	}

	// Set HEXPIRE for each field independently
	// HEXPIRE key seconds FIELDS N field1 field2 ...
	fields := []string{"id", "user_id", "messages", "turn_count", "max_turns", "created_at", "expires_at"}
	if len(intentJSON) > 0 {
		fields = append(fields, "intent")
	}
	if len(resultsJSON) > 0 {
		fields = append(fields, "results")
	}

	// Build doArgs: HEXPIRE key seconds FIELDS N field1 field2 ...
	doArgs := make([]interface{}, 0, 6+len(fields))
	doArgs = append(doArgs, "HEXPIRE", key, ttlSeconds, "FIELDS", strconv.Itoa(len(fields)))
	for _, f := range fields {
		doArgs = append(doArgs, f)
	}
	pipe.Do(ctx, doArgs...)

	if _, err := pipe.Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "save conversation: dragonfly pipeline failed",
			slog.String("conversation_id", conv.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("%w: save conversation: %w", ErrConversationStoreFailed, err)
	}

	// Publish event for async PG persistence via Dragonfly Streams.
	// The ConversationConsumer picks this up and calls pgStore.SaveConversationHistory.
	// Anonymous users (UserID == "") do NOT trigger events.
	//
	// FLAT PAYLOAD: go-redis XAdd requires all values to be marshalable as strings
	// or basic types (int, string). Nested map[string]interface{} is NOT marshalable.
	// We flatten all fields to the top level, matching the pattern in register/usecase.go.
	if conv.UserID != "" && eventBus != nil {
		messagesJSON, _ := json.Marshal(conv.Messages)
		intentJSON, _ := json.Marshal(conv.Intent)

		stream := eventbus.StreamName("search.conversation.saved")
		// Flat payload — estructura alineada con auth.user.registered.
		// timestamp: cuándo se creó la conversación (conv.CreatedAt), no time.Now()
		//            para evitar drift entre creación y publicación.
		// created_at: mismo dato en RFC3339 para legibilidad humana en debugging.
		// event_version: 1 — incrementar al cambiar estructura del payload.
		flatPayload := map[string]interface{}{
			"event_type":      string(eventbus.ConversationSaved),
			"event_version":   int64(1),
			"aggregate_id":    conv.ID,
			"timestamp":       conv.CreatedAt.UnixMilli(),
			"conversation_id": conv.ID,
			"user_id":         conv.UserID,
			"messages":        string(messagesJSON),
			"turn_count":      int64(conv.TurnCount),
			"max_turns":       int64(conv.MaxTurns),
			"created_at":      conv.CreatedAt.Format(time.RFC3339),
		}
		if len(intentJSON) > 0 {
			flatPayload["intent"] = string(intentJSON)
		}
		if len(conv.Results) > 0 {
			flatPayload["results"] = string(conv.Results)
		}
		if _, err := eventBus.Publish(context.WithoutCancel(ctx), stream, flatPayload); err != nil {
			slog.ErrorContext(ctx, "failed to publish conversation saved event",
				slog.String("conversation_id", conv.ID),
				slog.String("error", err.Error()),
			)
		}
	}

	return nil
}

// =============================================================================
// GetConversation
// =============================================================================

// GetConversation retrieves a ConversationState from Dragonfly.
// Returns nil, nil if the conversation does not exist (expired or never saved).
func GetConversation(ctx context.Context, rdb *redis.Client, conversationID string) (*domain.ConversationState, error) {
	if conversationID == "" {
		return nil, errors.New("conversation ID must not be empty")
	}

	key := conversationKey(conversationID)

	fields, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		slog.ErrorContext(ctx, "get conversation: dragonfly HGetAll failed",
			slog.String("conversation_id", conversationID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("%w: get conversation: %w", ErrConversationStoreFailed, err)
	}

	// Empty map means key doesn't exist or expired
	if len(fields) == 0 {
		slog.DebugContext(ctx, "get conversation: key not found or expired",
			slog.String("conversation_id", conversationID),
		)
		return nil, nil
	}

	// ID field must exist AND be non-empty. A key with empty value means
	// the conversation data was partially corrupted or the id field
	// was cleared (Dragonfly HEXPIRE edge case).
	if idVal, ok := fields["id"]; !ok || idVal == "" {
		slog.WarnContext(ctx, "get conversation: id field missing/empty (corrupted hash)",
			slog.String("conversation_id", conversationID),
			slog.Int("fields_count", len(fields)),
		)
		return nil, nil
	}

	slog.DebugContext(ctx, "get conversation: loaded hash fields",
		slog.String("conversation_id", conversationID),
		slog.Int("fields_count", len(fields)),
	)

	return parseConversationHash(fields)
}

// =============================================================================
// UpdateConversation
// =============================================================================

// UpdateConversation updates an existing conversation in Dragonfly.
// Resets the HEXPIRE TTL on all fields. The conv parameter must contain the
// full state — partial updates are NOT supported (call GetConversation first).
func UpdateConversation(ctx context.Context, rdb *redis.Client, conv *domain.ConversationState) error {
	if conv == nil {
		return errors.New("cannot update nil conversation")
	}
	if conv.ID == "" {
		return errors.New("conversation ID must not be empty")
	}

	// Check existence first
	key := conversationKey(conv.ID)
	exists, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		slog.ErrorContext(ctx, "update conversation: exists check failed",
			slog.String("conversation_id", conv.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("%w: exists check: %w", ErrConversationStoreFailed, err)
	}
	if exists == 0 {
		return fmt.Errorf("%w: conversation %s not found for update", ErrConversationStoreFailed, conv.ID)
	}

	// Reuse SaveConversation logic — full overwrite with TTL reset
	return SaveConversation(ctx, rdb, conv)
}

// =============================================================================
// Internal helpers
// =============================================================================

// parseConversationHash parses a Dragonfly Hash result map into a ConversationState.
// ALL errors are wrapped with ErrConversationStoreFailed so that the registered
// domain error mapper in module.go can map them to 503 (not 500).
func parseConversationHash(fields map[string]string) (*domain.ConversationState, error) {
	conv := &domain.ConversationState{
		ID:     fields["id"],
		UserID: fields["user_id"],
	}

	// Parse messages JSON
	if messagesStr := fields["messages"]; messagesStr != "" {
		if err := json.Unmarshal([]byte(messagesStr), &conv.Messages); err != nil {
			slog.Error("parseConversationHash: unmarshal messages failed",
				slog.String("conversation_id", conv.ID),
				slog.String("messages_raw", messagesStr[:min(len(messagesStr), 200)]),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("%w: unmarshal messages: %w", ErrConversationStoreFailed, err)
		}
	}

	// Parse intent JSON (optional)
	if intentStr := fields["intent"]; intentStr != "" {
		conv.Intent = &domain.TravelIntent{}
		if err := json.Unmarshal([]byte(intentStr), conv.Intent); err != nil {
			slog.Error("parseConversationHash: unmarshal intent failed",
				slog.String("conversation_id", conv.ID),
				slog.String("intent_raw", intentStr[:min(len(intentStr), 200)]),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("%w: unmarshal intent: %w", ErrConversationStoreFailed, err)
		}
	}

	// Parse results (optional — RawMessage)
	if resultsStr := fields["results"]; resultsStr != "" {
		conv.Results = json.RawMessage(resultsStr)
	}

	// Parse numeric fields
	if tc := fields["turn_count"]; tc != "" {
		v, err := strconv.Atoi(tc)
		if err != nil {
			slog.Error("parseConversationHash: parse turn_count failed",
				slog.String("conversation_id", conv.ID),
				slog.String("turn_count_raw", tc),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("%w: parse turn_count %q: %w", ErrConversationStoreFailed, tc, err)
		}
		conv.TurnCount = v
	}
	if mt := fields["max_turns"]; mt != "" {
		v, err := strconv.Atoi(mt)
		if err != nil {
			slog.Error("parseConversationHash: parse max_turns failed",
				slog.String("conversation_id", conv.ID),
				slog.String("max_turns_raw", mt),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("%w: parse max_turns %q: %w", ErrConversationStoreFailed, mt, err)
		}
		conv.MaxTurns = v
	}

	// Parse timestamps
	if ca := fields["created_at"]; ca != "" {
		t, err := time.Parse(time.RFC3339, ca)
		if err != nil {
			slog.Error("parseConversationHash: parse created_at failed",
				slog.String("conversation_id", conv.ID),
				slog.String("created_at_raw", ca),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("%w: parse created_at %q: %w", ErrConversationStoreFailed, ca, err)
		}
		conv.CreatedAt = t
	}
	if ea := fields["expires_at"]; ea != "" {
		t, err := time.Parse(time.RFC3339, ea)
		if err != nil {
			slog.Error("parseConversationHash: parse expires_at failed",
				slog.String("conversation_id", conv.ID),
				slog.String("expires_at_raw", ea),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("%w: parse expires_at %q: %w", ErrConversationStoreFailed, ea, err)
		}
		conv.ExpiresAt = t
	}

	slog.DebugContext(context.Background(), "parseConversationHash: success",
		slog.String("conversation_id", conv.ID),
		slog.Int("messages", len(conv.Messages)),
		slog.Int("turn_count", conv.TurnCount),
	)

	return conv, nil
}

// =============================================================================
// Event bus integration — event-driven PG persistence
// =============================================================================

// eventBus is wired by InitEventBus() during module initialization.
// When set, SaveConversation publishes "{events}:search.conversation.saved" events
// for authenticated users. The ConversationConsumer picks these up and calls
// pgStore.SaveConversationHistory() asynchronously.
var eventBus *eventbus.EventBus

// InitEventBus wires the shared EventBus into the conversation package.
// Called once during module initialization in module.go.
func InitEventBus(eb *eventbus.EventBus) {
	eventBus = eb
}
