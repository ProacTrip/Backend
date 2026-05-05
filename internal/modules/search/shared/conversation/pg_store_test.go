// pg_store_test.go — tests for PostgreSQL conversation persistence.
// Uses a mock PgxPool to verify query execution without a real DB.
package conversation

import (
	"context"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/jackc/pgx/v5/pgconn"
)

// =============================================================================
// Mock PgxPool
// =============================================================================

// mockPgxPool implements PgxPool for testing PG store behavior.
type mockPgxPool struct {
	execCalled int
	lastSQL    string
	lastArgs   []any
	execErr    error
}

func (m *mockPgxPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.execCalled++
	m.lastSQL = sql
	m.lastArgs = args
	if m.execErr != nil {
		return pgconn.CommandTag{}, m.execErr
	}
	return pgconn.CommandTag{}, nil
}

// =============================================================================
// Test helper
// =============================================================================

func newTestConv() *domain.ConversationState {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.ConversationState{
		ID:        "conv-pg-test-001",
		UserID:    "user-auth-pg-001",
		Messages: []domain.ConversationMessage{
			{Role: "user", Content: "busco hotel en Barcelona", Timestamp: now},
			{Role: "assistant", Content: "¿Qué fechas?", Timestamp: now.Add(time.Second)},
		},
		Intent: &domain.TravelIntent{
			Type:       "hotels",
			Confidence: 0.88,
			HotelParams: &domain.HotelSearchRequest{
				Query:        "Barcelona",
				CheckInDate:  "2026-07-01",
				CheckOutDate: "2026-07-05",
				Adults:       2,
			},
		},
		TurnCount: 2,
		MaxTurns:  10,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	}
}

// =============================================================================
// Test: SaveConversationHistory — authenticated user triggers PG write
// =============================================================================

func TestPgSaveConversationHistory_AuthUser_ExecutesUpsert(t *testing.T) {
	ctx := t.Context()
	pool := &mockPgxPool{}
	store := NewPgConversationStore(pool)

	conv := newTestConv()
	store.SaveConversationHistory(ctx, conv)

	if pool.execCalled != 1 {
		t.Fatalf("expected 1 Exec call, got %d", pool.execCalled)
	}

	// Verify upsert SQL was used
	if pool.lastSQL == "" {
		t.Error("expected non-empty SQL statement")
	}

	// Verify conversation_id is the first arg
	if len(pool.lastArgs) < 1 || pool.lastArgs[0] != conv.ID {
		t.Errorf("expected first arg to be conversation_id %q, got %v", conv.ID, pool.lastArgs[0])
	}

	// Verify user_id is the second arg
	if len(pool.lastArgs) < 2 || pool.lastArgs[1] != conv.UserID {
		t.Errorf("expected second arg to be user_id %q, got %v", conv.UserID, pool.lastArgs[1])
	}
}

// =============================================================================
// Test: SaveConversationHistory — anonymous user skips PG write
// =============================================================================

func TestPgSaveConversationHistory_AnonymousUser_SkipsExec(t *testing.T) {
	ctx := t.Context()
	pool := &mockPgxPool{}
	store := NewPgConversationStore(pool)

	conv := newTestConv()
	conv.UserID = "" // anonymous

	store.SaveConversationHistory(ctx, conv)

	// Exec should NOT be called for anonymous users
	if pool.execCalled != 0 {
		t.Errorf("expected 0 Exec calls for anonymous user, got %d", pool.execCalled)
	}
}

// =============================================================================
// Test: SaveConversationHistory — nil conversation is no-op
// =============================================================================

func TestPgSaveConversationHistory_NilConversation_Noop(t *testing.T) {
	ctx := t.Context()
	pool := &mockPgxPool{}
	store := NewPgConversationStore(pool)

	store.SaveConversationHistory(ctx, nil)

	if pool.execCalled != 0 {
		t.Errorf("expected 0 Exec calls for nil conversation, got %d", pool.execCalled)
	}
}

// =============================================================================
// Test: SaveConversationHistory — nil intent handled gracefully
// =============================================================================

func TestPgSaveConversationHistory_NilIntent_ExecutesWithNullJSON(t *testing.T) {
	ctx := t.Context()
	pool := &mockPgxPool{}
	store := NewPgConversationStore(pool)

	conv := newTestConv()
	conv.Intent = nil // no intent (happens before first AI interpretation)

	store.SaveConversationHistory(ctx, conv)

	if pool.execCalled != 1 {
		t.Fatalf("expected 1 Exec call, got %d", pool.execCalled)
	}
}

// =============================================================================
// Test: SaveConversationHistory — DB error handled gracefully
// =============================================================================

func TestPgSaveConversationHistory_ExecError_NoPanic(t *testing.T) {
	ctx := t.Context()
	pool := &mockPgxPool{
		execErr: context.DeadlineExceeded,
	}
	store := NewPgConversationStore(pool)

	conv := newTestConv()

	// Should NOT panic on DB error — just logs and returns
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SaveConversationHistory panicked on DB error: %v", r)
			}
		}()
		store.SaveConversationHistory(ctx, conv)
	}()

	if pool.execCalled != 1 {
		t.Fatalf("expected 1 Exec call, got %d", pool.execCalled)
	}
}

// =============================================================================
// Test: InitEventBus wires the event bus into the conversation package
// =============================================================================

func TestInitEventBus_WiresEventBus(t *testing.T) {
	// eventBus should be nil before InitEventBus
	prev := eventBus
	eventBus = nil

	// Create a fake EventBus — only the pointer matters for wiring
	eb := &eventbus.EventBus{}
	InitEventBus(eb)

	if eventBus == nil {
		t.Error("InitEventBus() should wire eventBus, but it's still nil")
	}
	if eventBus != eb {
		t.Error("InitEventBus() should store the passed EventBus pointer")
	}

	// Restore original value
	eventBus = prev
}

// =============================================================================
// Test: TurnCount and MaxTurns propagated to upsert args
// =============================================================================

func TestPgSaveConversationHistory_TurnCountsInArgs(t *testing.T) {
	ctx := t.Context()
	pool := &mockPgxPool{}
	store := NewPgConversationStore(pool)

	conv := newTestConv()
	conv.TurnCount = 7
	conv.MaxTurns = 15

	store.SaveConversationHistory(ctx, conv)

	if pool.execCalled != 1 {
		t.Fatalf("expected 1 Exec call, got %d", pool.execCalled)
	}

	// The args list: conversation_id, user_id, messages, intent, results, turn_count, max_turns, created_at
	// turn_count is at index 5, max_turns at index 6
	if len(pool.lastArgs) < 7 {
		t.Fatalf("expected at least 7 args, got %d", len(pool.lastArgs))
	}

	// turn_count (index 5)
	if tc, ok := pool.lastArgs[5].(int); !ok || tc != 7 {
		t.Errorf("expected turn_count=7 at arg[5], got %v", pool.lastArgs[5])
	}

	// max_turns (index 6)
	if mt, ok := pool.lastArgs[6].(int); !ok || mt != 15 {
		t.Errorf("expected max_turns=15 at arg[6], got %v", pool.lastArgs[6])
	}
}
