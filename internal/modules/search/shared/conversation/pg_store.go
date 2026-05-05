// pg_store — PostgreSQL conversation persistence via event-driven streams.
//
// DESIGN: SaveConversationHistory is called by ConversationConsumer when it
// receives a "{events}:search.conversation.saved" event from Dragonfly Streams.
// Only persists authenticated users (UserID != "").
// Uses context.WithoutCancel() so the consumer's context cancellation
// does not abort the PG write — the conversation state must be durably stored.
package conversation

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

// =============================================================================
// PgConversationStore
// =============================================================================

// PgxPool is the minimal pgx pool interface needed for conversation persistence.
// Mirrors the PgxPool interface in adapters/postgres/search_history_repo.go
// to avoid an import cycle.
type PgxPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// PgConversationStore handles PostgreSQL writes for conversation state.
// Only stores conversations from authenticated users (UserID != "").
// Called by the ConversationConsumer when it receives conversation_saved events.
type PgConversationStore struct {
	pool PgxPool
}

// NewPgConversationStore creates a new PostgreSQL conversation store.
// Wire this in bootstrap/app.go with the pgx pool instance.
func NewPgConversationStore(pool PgxPool) *PgConversationStore {
	return &PgConversationStore{pool: pool}
}

// =============================================================================
// SaveConversationHistory — event-driven PG write
// =============================================================================

// SaveConversationHistory persists a conversation state to PostgreSQL.
// Called by the ConversationConsumer when processing conversation_saved events.
// Uses context.WithoutCancel() internally so the consumer's context cancellation
// does not interrupt the write.
//
// Table: ai_conversations
//   - conversation_id UUID PK (from conv.ID)
//   - user_id UUID (from conv.UserID)
//   - messages JSONB (serialized ConversationMessage array)
//   - intent JSONB (serialized TravelIntent, nullable)
//   - results JSONB (raw results payload, nullable)
//   - turn_count INT
//   - max_turns INT
//   - created_at TIMESTAMPTZ
//   - updated_at TIMESTAMPTZ
func (s *PgConversationStore) SaveConversationHistory(ctx context.Context, conv *domain.ConversationState) {
	if conv == nil || conv.UserID == "" {
		return // only persist auth users
	}

	// Use context.WithoutCancel to prevent upstream cancellation
	// from aborting the PG write.
	ctx = context.WithoutCancel(ctx)

	messagesJSON, err := json.Marshal(conv.Messages)
	if err != nil {
		slog.Error("pg conversation store: marshal messages failed",
			slog.String("conversation_id", conv.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	intentJSON, err := json.Marshal(conv.Intent)
	if err != nil {
		slog.Error("pg conversation store: marshal intent failed",
			slog.String("conversation_id", conv.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	resultsJSON := []byte(conv.Results)
	if len(resultsJSON) == 0 {
		resultsJSON = []byte("null")
	}

	// Upsert: INSERT ... ON CONFLICT UPDATE
	// This ensures repeated saves for the same conversation don't create duplicates.
	_, err = s.pool.Exec(ctx, `
		INSERT INTO ai_conversations
			(conversation_id, user_id, messages, intent, results, turn_count, max_turns, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (conversation_id) DO UPDATE SET
			messages = EXCLUDED.messages,
			intent = EXCLUDED.intent,
			results = EXCLUDED.results,
			turn_count = EXCLUDED.turn_count,
			updated_at = NOW()
	`, conv.ID, conv.UserID, messagesJSON, intentJSON, resultsJSON,
		conv.TurnCount, conv.MaxTurns, conv.CreatedAt)

	if err != nil {
		slog.Error("pg conversation store: upsert failed",
			slog.String("conversation_id", conv.ID),
			slog.String("user_id", conv.UserID),
			slog.String("error", err.Error()),
		)
		return
	}

	slog.Debug("pg conversation store: saved",
		slog.String("conversation_id", conv.ID),
		slog.Int("turn_count", conv.TurnCount),
	)
}
