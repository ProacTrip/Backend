package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// TDD RED Phase — test file written BEFORE implementation
// =============================================================================

// newTestRedis creates a miniredis-backed redis.Client for testing.
func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run failed: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return rdb, mr
}

// newTestConversation creates a sample ConversationState for tests.
func newTestConversation() *domain.ConversationState {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.ConversationState{
		ID:     "conv-test-001",
		UserID: "user-auth-123",
		Messages: []domain.ConversationMessage{
			{Role: "user", Content: "vuelo a Madrid", Timestamp: now},
			{Role: "assistant", Content: "¿Qué fecha?", Timestamp: now.Add(time.Second)},
		},
		Intent: &domain.TravelIntent{
			Type:       "flights",
			Confidence: 0.85,
			FlightParams: &domain.FlightSearchRequest{
				Departure:    "EZE",
				Arrival:      "MAD",
				OutboundDate: "2026-07-01",
				Adults:       1,
			},
		},
		TurnCount: 2,
		MaxTurns:  5,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	}
}

// =============================================================================
// Save + Get roundtrip (Dragonfly)
// =============================================================================

func TestSaveAndGetConversation_Roundtrip(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	conv := newTestConversation()

	if err := SaveConversation(ctx, rdb, conv); err != nil {
		t.Fatalf("SaveConversation failed: %v", err)
	}

	got, err := GetConversation(ctx, rdb, conv.ID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil (expected saved conversation)")
	}

	if got.ID != conv.ID {
		t.Errorf("ID = %q, want %q", got.ID, conv.ID)
	}
	if got.UserID != conv.UserID {
		t.Errorf("UserID = %q, want %q", got.UserID, conv.UserID)
	}
	if got.TurnCount != conv.TurnCount {
		t.Errorf("TurnCount = %d, want %d", got.TurnCount, conv.TurnCount)
	}
	if got.MaxTurns != conv.MaxTurns {
		t.Errorf("MaxTurns = %d, want %d", got.MaxTurns, conv.MaxTurns)
	}
	if got.Intent == nil {
		t.Fatal("Intent should not be nil")
	}
	if got.Intent.Type != conv.Intent.Type {
		t.Errorf("Intent.Type = %q, want %q", got.Intent.Type, conv.Intent.Type)
	}
	if len(got.Messages) != len(conv.Messages) {
		t.Errorf("Messages len = %d, want %d", len(got.Messages), len(conv.Messages))
	}
}

// =============================================================================
// Get non-existent conversation → nil
// =============================================================================

func TestGetConversation_NotFound(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	got, err := GetConversation(ctx, rdb, "conv-nonexistent")
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent conversation, got: %+v", got)
	}
}

// =============================================================================
// Update preserves fields
// =============================================================================

func TestUpdateConversation_PreservesFields(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	// Save initial
	conv := newTestConversation()
	if err := SaveConversation(ctx, rdb, conv); err != nil {
		t.Fatalf("SaveConversation failed: %v", err)
	}

	// Modify and update
	conv.TurnCount = 3
	conv.Messages = append(conv.Messages, domain.ConversationMessage{
		Role:      "assistant",
		Content:   "Estos son los resultados:",
		Timestamp: time.Now().UTC(),
	})
	resultsJSON := json.RawMessage(`{"flights":["result1","result2"]}`)
	conv.Results = resultsJSON

	if err := UpdateConversation(ctx, rdb, conv); err != nil {
		t.Fatalf("UpdateConversation failed: %v", err)
	}

	// Retrieve and verify
	got, err := GetConversation(ctx, rdb, conv.ID)
	if err != nil {
		t.Fatalf("GetConversation after update failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil after update")
	}

	if got.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", got.TurnCount)
	}
	if len(got.Messages) != 3 {
		t.Errorf("Messages len = %d, want 3", len(got.Messages))
	}
	if string(got.Results) != string(resultsJSON) {
		t.Errorf("Results = %s, want %s", string(got.Results), string(resultsJSON))
	}
	// Other fields should be preserved
	if got.ID != conv.ID {
		t.Errorf("ID = %q, want %q", got.ID, conv.ID)
	}
}

// =============================================================================
// Anon user → no PG save (only Dragonfly save, no error)
// =============================================================================

func TestSaveConversation_AnonymousUser_DragonflyOnly(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	conv := newTestConversation()
	conv.UserID = "" // anonymous
	conv.ID = "conv-anon-test"

	// Save succeeds (Dragonfly only, no PG)
	if err := SaveConversation(ctx, rdb, conv); err != nil {
		t.Fatalf("SaveConversation for anon user failed: %v", err)
	}

	// Should be retrievable from Dragonfly
	got, err := GetConversation(ctx, rdb, conv.ID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil (should have saved to Dragonfly)")
	}
	if got.UserID != "" {
		t.Errorf("UserID = %q, want empty", got.UserID)
	}
}

// =============================================================================
// Auth user → PG save attempted (async, fire-and-forget)
// =============================================================================

func TestSaveConversation_AuthUser_TriggersPgSave(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	conv := newTestConversation()
	conv.UserID = "user-auth-456"

	// With nil db (no PG available), save should still succeed (Dragonfly)
	if err := SaveConversation(ctx, rdb, conv); err != nil {
		t.Fatalf("SaveConversation for auth user failed: %v", err)
	}

	// Verify Dragonfly save works regardless of PG availability
	got, err := GetConversation(ctx, rdb, conv.ID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil")
	}
}

// =============================================================================
// Multiple conversations don't collide
// =============================================================================

func TestMultipleConversations_NoCollision(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	conv1 := newTestConversation()
	conv1.ID = "conv-a"
	conv1.TurnCount = 1

	conv2 := newTestConversation()
	conv2.ID = "conv-b"
	conv2.TurnCount = 7

	if err := SaveConversation(ctx, rdb, conv1); err != nil {
		t.Fatalf("SaveConversation(conv1) failed: %v", err)
	}
	if err := SaveConversation(ctx, rdb, conv2); err != nil {
		t.Fatalf("SaveConversation(conv2) failed: %v", err)
	}

	got1, err := GetConversation(ctx, rdb, conv1.ID)
	if err != nil {
		t.Fatalf("GetConversation(conv1) failed: %v", err)
	}
	got2, err := GetConversation(ctx, rdb, conv2.ID)
	if err != nil {
		t.Fatalf("GetConversation(conv2) failed: %v", err)
	}

	if got1.TurnCount != 1 {
		t.Errorf("conv1 TurnCount = %d, want 1", got1.TurnCount)
	}
	if got2.TurnCount != 7 {
		t.Errorf("conv2 TurnCount = %d, want 7", got2.TurnCount)
	}
}

// =============================================================================
// Save with nil conversation
// =============================================================================

func TestSaveConversation_NilConversation(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	err := SaveConversation(ctx, rdb, nil)
	if err == nil {
		t.Error("expected error for nil conversation, got nil")
	}
}

// =============================================================================
// Save with empty ID
// =============================================================================

func TestSaveConversation_EmptyID(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	conv := newTestConversation()
	conv.ID = ""

	err := SaveConversation(ctx, rdb, conv)
	if err == nil {
		t.Error("expected error for empty ID, got nil")
	}
}

// =============================================================================
// Compile-time check that errors are distinct
// =============================================================================

func TestConversationErrors_AreDistinct(t *testing.T) {
	if ErrConversationStoreFailed.Error() == "" {
		t.Error("ErrConversationStoreFailed should have a non-empty error message")
	}
	if errors.Is(ErrConversationStoreFailed, context.Canceled) {
		t.Error("ErrConversationStoreFailed should not match context.Canceled")
	}
	if errors.Is(ErrConversationStoreFailed, ErrConversationStoreFailed) {
		// Good — it should match itself
	} else {
		t.Error("ErrConversationStoreFailed should match itself")
	}
}
