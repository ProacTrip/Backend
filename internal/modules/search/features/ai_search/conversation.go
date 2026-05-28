// Conversation persistence for AI search multi-turn sessions.
//
// DESIGN: DragonflyDB JSON-string storage with hashtag key routing.
//   - Key format: {conv}:{id} (hashtag ensures same shard for pipeline ops)
//   - User index: user:convs:{user_id} (SMEMBERS for listing)
//   - TTL: 300s (5 min), reset on every POST activity
//   - Keyspace notifications: __keyevent@0__:expired → SSE event
package ai_search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Constants
// =============================================================================

const (
	// conversationKeyPrefix ensures co-location on the same Dragonfly shard
	// for pipeline operations (SET + SADD). The hashtag {conv} is the shard key.
	conversationKeyPrefix = "{conv}:"

	// userConvsKeyPrefix is the set key for listing a user's active conversations.
	userConvsKeyPrefix = "user:convs:"

	// convOwnerKeyPrefix is the reverse-mapping key for resolving
	// a userID from a conversation ID during expiry notifications.
	convOwnerKeyPrefix = "conv:owner:"

	// conversationTTL is the TTL for conversation keys in DragonflyDB.
	// Resets on every POST activity. GET reads do NOT reset.
	conversationTTL = 5 * time.Minute
)

// =============================================================================
// Key helpers
// =============================================================================

// convKey builds the Dragonfly key for a conversation.
func convKey(id string) string {
	return fmt.Sprintf("%s%s", conversationKeyPrefix, id)
}

// userConvsKey builds the Dragonfly set key for a user's conversations.
func userConvsKey(userID string) string {
	return fmt.Sprintf("%s%s", userConvsKeyPrefix, userID)
}

// convOwnerKey builds the reverse-mapping key to resolve a userID
// from a conversation ID during expiry notifications.
func convOwnerKey(convID string) string {
	return fmt.Sprintf("%s%s", convOwnerKeyPrefix, convID)
}

// =============================================================================
// Conversation — full conversation state
// =============================================================================

// Conversation represents the complete state of a multi-turn AI search chat.
type Conversation struct {
	ID          string                `json:"id"`
	UserID      string                `json:"user_id"` // "" for anonymous
	Messages    []domain.ConversationMessage `json:"messages"`
	SearchCache map[string]*CachedSearch     `json:"search_cache,omitzero"`
	Filters     FilterState            `json:"filters,omitzero"`
	Context     ConversationContext    `json:"context,omitzero"`
	TurnCount   int                   `json:"turn_count"`
	MaxTurns    int                   `json:"max_turns"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

// ConversationPreview is a lightweight summary for listing conversations.
type ConversationPreview struct {
	ID        string    `json:"id"`
	Preview   string    `json:"preview"` // first user message
	TurnCount int       `json:"turn_count"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CachedSearch holds search results for a single tool call.
type CachedSearch struct {
	Response    json.RawMessage `json:"response"`
	Destination string          `json:"destination,omitzero"`
	Type        string          `json:"type"` // "hotels" or "flights"
}

// FilterState holds the current active and available filters.
type FilterState struct {
	Hotels  map[string]interface{} `json:"hotels,omitzero"`
	Flights map[string]interface{} `json:"flights,omitzero"`
}

// ConversationContext holds resolved location/preference context.
type ConversationContext struct {
	Location    string `json:"location,omitzero"`
	CountryCode string `json:"country_code,omitzero"`
	Currency    string `json:"currency,omitzero"`
	Language    string `json:"language,omitzero"`
}

// =============================================================================
// ConversationStore — DragonflyDB-backed persistence
// =============================================================================

// ConvStore provides CRUD operations for conversation persistence
// backed by DragonflyDB JSON-string keys ({conv}:{id}).
//
// This is the CONCRETE implementation. The ConversationStore interface
// in usecase.go is the abstraction that use cases depend on.
type ConvStore struct {
	rdb *redis.Client
}

// NewConvStore creates a new ConvStore with a DragonflyDB client.
func NewConvStore(rdb *redis.Client) *ConvStore {
	return &ConvStore{rdb: rdb}
}

// Save persists a Conversation to DragonflyDB with TTL and user index.
func (s *ConvStore) Save(ctx context.Context, conv *Conversation) error {
	if conv == nil {
		return fmt.Errorf("cannot save nil conversation")
	}
	if conv.ID == "" {
		return fmt.Errorf("conversation ID must not be empty")
	}

	now := time.Now()
	conv.UpdatedAt = now

	data, err := json.Marshal(conv)
	if err != nil {
		return fmt.Errorf("marshal conversation %s: %w", conv.ID, err)
	}

	key := convKey(conv.ID)
	ttlSeconds := int(conversationTTL.Seconds())

	// Pipeline: SET + SADD + EXPIRE (atomic within same hashtag shard)
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, key, string(data), conversationTTL)

	if conv.UserID != "" {
		pipe.SAdd(ctx, userConvsKey(conv.UserID), conv.ID)
		pipe.Expire(ctx, userConvsKey(conv.UserID), conversationTTL)

		// Reverse mapping: conv:owner:{convID} → userID for expiry notifications.
		// Stored with the same TTL so it expires together with the conversation.
		pipe.Set(ctx, convOwnerKey(conv.ID), conv.UserID, conversationTTL)
	} else {
		slog.DebugContext(ctx, "ConversationStore.Save: skipping user index for anonymous conversation",
			slog.String("conversation_id", conv.ID),
		)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "ConversationStore.Save: pipeline failed",
			slog.String("conversation_id", conv.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("save conversation %s: %w", conv.ID, err)
	}

	slog.DebugContext(ctx, "ConversationStore.Save: saved",
		slog.String("conversation_id", conv.ID),
		slog.Int("ttl_seconds", ttlSeconds),
	)

	_ = ttlSeconds
	return nil
}

// Load retrieves a Conversation from DragonflyDB.
// Returns nil, nil if the conversation does not exist (expired or never saved).
func (s *ConvStore) Load(ctx context.Context, convID string) (*Conversation, error) {
	if convID == "" {
		return nil, fmt.Errorf("conversation ID must not be empty")
	}

	key := convKey(convID)
	raw, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		slog.ErrorContext(ctx, "ConversationStore.Load: GET failed",
			slog.String("conversation_id", convID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("load conversation %s: %w", convID, err)
	}

	var conv Conversation
	if err := json.Unmarshal([]byte(raw), &conv); err != nil {
		slog.ErrorContext(ctx, "ConversationStore.Load: unmarshal failed",
			slog.String("conversation_id", convID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("unmarshal conversation %s: %w", convID, err)
	}

	return &conv, nil
}

// Delete removes a conversation from DragonflyDB and the user index.
func (s *ConvStore) Delete(ctx context.Context, convID, userID string) error {
	if convID == "" {
		return fmt.Errorf("conversation ID must not be empty")
	}

	key := convKey(convID)

	pipe := s.rdb.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, userConvsKey(userID), convID)
	// REQ-W4: Clean up reverse-mapping key to prevent stale SSE events
	// for deleted conversations.
	pipe.Del(ctx, convOwnerKey(convID))

	if _, err := pipe.Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "ConversationStore.Delete: pipeline failed",
			slog.String("conversation_id", convID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("delete conversation %s: %w", convID, err)
	}

	slog.DebugContext(ctx, "ConversationStore.Delete: deleted",
		slog.String("conversation_id", convID),
	)

	return nil
}

// ListUserConversations returns active conversation previews for a user.
func (s *ConvStore) ListUserConversations(ctx context.Context, userID string) ([]ConversationPreview, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID must not be empty")
	}

	// Get conversation IDs from the user's set
	convIDs, err := s.rdb.SMembers(ctx, userConvsKey(userID)).Result()
	if err != nil {
		if err == redis.Nil {
			return []ConversationPreview{}, nil
		}
		slog.ErrorContext(ctx, "ConversationStore.ListUserConversations: SMEMBERS failed",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("list conversations for user %s: %w", userID, err)
	}

	if len(convIDs) == 0 {
		return []ConversationPreview{}, nil
	}

	// MGET all conversation previews
	keys := make([]string, len(convIDs))
	for i, id := range convIDs {
		keys[i] = convKey(id)
	}

	rawValues, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		slog.ErrorContext(ctx, "ConversationStore.ListUserConversations: MGET failed",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("mget conversations: %w", err)
	}

	previews := make([]ConversationPreview, 0, len(rawValues))
	for i, raw := range rawValues {
		if raw == nil {
			// Conversation expired between SMEMBERS and MGET — clean up
			s.rdb.SRem(ctx, userConvsKey(userID), convIDs[i])
			continue
		}

		rawStr, ok := raw.(string)
		if !ok {
			continue
		}

		var conv Conversation
		if err := json.Unmarshal([]byte(rawStr), &conv); err != nil {
			continue
		}

		// Extract preview (first user message)
		preview := ""
		for _, msg := range conv.Messages {
			if msg.Role == "user" {
				preview = msg.Content
				break
			}
		}

		previews = append(previews, ConversationPreview{
			ID:        conv.ID,
			Preview:   preview,
			TurnCount: conv.TurnCount,
			UpdatedAt: conv.UpdatedAt,
		})
	}

	return previews, nil
}

// ResetTTL resets the TTL on a conversation key to 300 seconds.
func (s *ConvStore) ResetTTL(ctx context.Context, convID string) error {
	if convID == "" {
		return fmt.Errorf("conversation ID must not be empty")
	}

	key := convKey(convID)
	ttlSeconds := int(conversationTTL.Seconds())

	if err := s.rdb.Expire(ctx, key, conversationTTL).Err(); err != nil {
		slog.ErrorContext(ctx, "ConversationStore.ResetTTL: EXPIRE failed",
			slog.String("conversation_id", convID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("reset ttl for conversation %s: %w", convID, err)
	}

	slog.DebugContext(ctx, "ConversationStore.ResetTTL: TTL reset",
		slog.String("conversation_id", convID),
		slog.Int("ttl_seconds", ttlSeconds),
	)

	return nil
}
