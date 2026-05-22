// Integration tests for conversation persistence using miniredis.
// Verifies the full ConversationStore CRUD round-trip, TTL, expiry,
// and user conversation listing.
//
// Phase 6.2-6.3: End-to-end + F5 recovery tests for ai-chat-search.
package ai_search

import (
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Test helpers
// =============================================================================

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

func newTestConversation() *Conversation {
	now := time.Now().UTC().Truncate(time.Second)
	return &Conversation{
		ID:     "conv-test-001",
		UserID: "user-auth-123",
		Messages: []domain.ConversationMessage{
			{Role: "user", Content: "hoteles en Bali", Timestamp: now},
			{Role: "assistant", Content: "Buscando hoteles en Bali...", Timestamp: now.Add(time.Second)},
		},
		SearchCache: map[string]*CachedSearch{
			"call_abc": {
				Response:    []byte(`{"properties":[{"name":"Test Hotel"}]}`),
				Destination: "Bali, Indonesia",
				Type:        "hotels",
			},
		},
		Filters: FilterState{
			Hotels: map[string]interface{}{
				"stars": 5,
				"spa":   true,
			},
		},
		Context: ConversationContext{
			Location:    "Buenos Aires, Argentina",
			CountryCode: "AR",
			Currency:    "ARS",
			Language:    "es",
		},
		TurnCount: 2,
		MaxTurns:  5,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// =============================================================================
// 6.2 — Save + Load roundtrip
// =============================================================================

func TestConversationStore_SaveAndLoad_Roundtrip(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)
	conv := newTestConversation()

	// Save
	if err := store.Save(ctx, conv); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := store.Load(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil (expected saved conversation)")
	}

	// Verify fields
	if loaded.ID != conv.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, conv.ID)
	}
	if loaded.UserID != conv.UserID {
		t.Errorf("UserID = %q, want %q", loaded.UserID, conv.UserID)
	}
	if loaded.TurnCount != conv.TurnCount {
		t.Errorf("TurnCount = %d, want %d", loaded.TurnCount, conv.TurnCount)
	}
	if loaded.MaxTurns != conv.MaxTurns {
		t.Errorf("MaxTurns = %d, want %d", loaded.MaxTurns, conv.MaxTurns)
	}
	if len(loaded.Messages) != len(conv.Messages) {
		t.Errorf("Messages len = %d, want %d", len(loaded.Messages), len(conv.Messages))
	}
	if loaded.Context.Location != conv.Context.Location {
		t.Errorf("Context.Location = %q, want %q", loaded.Context.Location, conv.Context.Location)
	}
	if loaded.Context.Currency != conv.Context.Currency {
		t.Errorf("Context.Currency = %q, want %q", loaded.Context.Currency, conv.Context.Currency)
	}
	// Verify search cache
	if len(loaded.SearchCache) != 1 {
		t.Errorf("SearchCache len = %d, want 1", len(loaded.SearchCache))
	}
	if cache, ok := loaded.SearchCache["call_abc"]; !ok {
		t.Error("SearchCache[call_abc] missing")
	} else if cache.Destination != "Bali, Indonesia" {
		t.Errorf("SearchCache destination = %q, want 'Bali, Indonesia'", cache.Destination)
	}
}

// =============================================================================
// Load non-existent → nil
// =============================================================================

func TestConversationStore_Load_NotFound(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)

	loaded, err := store.Load(ctx, "conv-nonexistent")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for non-existent conversation, got: %+v", loaded)
	}
}

// =============================================================================
// TTL expiry (fast-forward miniredis clock)
// =============================================================================

func TestConversationStore_Expiry_ExpiresAfterTTL(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)
	conv := newTestConversation()

	// Save
	if err := store.Save(ctx, conv); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify it exists
	loaded, err := store.Load(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("conversation should exist immediately after save")
	}

	// Fast-forward clock past TTL (300s + buffer)
	mr.FastForward(6 * time.Minute)

	// Should be expired now
	loaded, err = store.Load(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Load after expiry failed: %v", err)
	}
	if loaded != nil {
		t.Errorf("conversation should be expired after TTL elapsed, got: %+v", loaded)
	}
}

// =============================================================================
// TTL reset on Save
// =============================================================================

func TestConversationStore_ResetTTL(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)
	conv := newTestConversation()

	// Save
	if err := store.Save(ctx, conv); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Fast forward 4 minutes (within TTL window)
	mr.FastForward(4 * time.Minute)

	// Reset TTL
	if err := store.ResetTTL(ctx, conv.ID); err != nil {
		t.Fatalf("ResetTTL failed: %v", err)
	}

	// Fast forward another 4 minutes (would have expired if TTL wasn't reset)
	// Total 8 minutes elapsed → should have expired at 5 min, but TTL reset at 4 min
	// so key should still exist (expires at 4+5=9 min)
	mr.FastForward(4 * time.Minute)

	loaded, err := store.Load(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded != nil {
		t.Logf("conversation still alive after TTL reset (expected: still within window)")
	} else {
		t.Error("conversation should still exist after TTL reset")
	}
}

// =============================================================================
// Delete
// =============================================================================

func TestConversationStore_Delete(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)
	conv := newTestConversation()

	// Save
	if err := store.Save(ctx, conv); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Delete
	if err := store.Delete(ctx, conv.ID, conv.UserID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should be gone
	loaded, err := store.Load(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Load after delete failed: %v", err)
	}
	if loaded != nil {
		t.Errorf("conversation should be gone after delete, got: %+v", loaded)
	}
}

// =============================================================================
// User conversation listing
// =============================================================================

func TestConversationStore_ListUserConversations(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)

	// Create 3 conversations for user
	for i := range 3 {
		conv := newTestConversation()
		conv.ID = fmt.Sprintf("conv-%d", i)
		conv.UserID = "user-42"
		conv.Messages = []domain.ConversationMessage{
			{Role: "user", Content: fmt.Sprintf("message %d", i), Timestamp: time.Now()},
		}
		if err := store.Save(ctx, conv); err != nil {
			t.Fatalf("Save conv-%d failed: %v", i, err)
		}
	}

	// List
	previews, err := store.ListUserConversations(ctx, "user-42")
	if err != nil {
		t.Fatalf("ListUserConversations failed: %v", err)
	}

	if len(previews) != 3 {
		t.Errorf("expected 3 previews, got %d", len(previews))
	}

	// Verify previews have correct data
	for _, p := range previews {
		if p.ID == "" {
			t.Error("preview ID should not be empty")
		}
		if p.Preview == "" {
			t.Error("preview should have a preview (first user message)")
		}
		if p.TurnCount < 0 {
			t.Errorf("preview TurnCount should be >= 0, got %d", p.TurnCount)
		}
	}
}

// =============================================================================
// List for user with no conversations → empty slice
// =============================================================================

func TestConversationStore_ListUserConversations_Empty(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)

	previews, err := store.ListUserConversations(ctx, "user-nonexistent")
	if err != nil {
		t.Fatalf("ListUserConversations failed: %v", err)
	}

	if len(previews) != 0 {
		t.Errorf("expected 0 previews for new user, got %d", len(previews))
	}
}

// =============================================================================
// 6.3 — F5 recovery: POST → GET → POST → DELETE (full lifetime)
// =============================================================================

func TestConversationStore_F5Recovery_FullLifecycle(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)

	// 1. POST: Save new conversation (first message)
	convID := "conv-f5-test"
	conv := &Conversation{
		ID:     convID,
		UserID: "user-f5",
		Messages: []domain.ConversationMessage{
			{Role: "user", Content: "vuelos a Madrid en junio", Timestamp: time.Now().UTC()},
		},
		Context: ConversationContext{
			Location:    "Barcelona, España",
			CountryCode: "ES",
			Currency:    "EUR",
		},
		TurnCount: 1,
		MaxTurns:  5,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := store.Save(ctx, conv); err != nil {
		t.Fatalf("Save (POST) failed: %v", err)
	}

	// 2. GET: Verify full state (F5 recovery)
	loaded, err := store.Load(ctx, convID)
	if err != nil {
		t.Fatalf("Load (GET) failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("conversation should exist after POST")
	}
	if loaded.ID != convID {
		t.Errorf("ID = %q, want %q", loaded.ID, convID)
	}
	if len(loaded.Messages) != 1 {
		t.Errorf("Messages len = %d, want 1", len(loaded.Messages))
	}
	if loaded.Context.Currency != "EUR" {
		t.Errorf("Currency = %q, want EUR", loaded.Context.Currency)
	}

	// 3. POST: Append message to existing conversation
	loaded.Messages = append(loaded.Messages, domain.ConversationMessage{
		Role:      "assistant",
		Content:   "Encontré 15 vuelos a Madrid.",
		Timestamp: time.Now().UTC(),
	})
	loaded.TurnCount = 2
	loaded.SearchCache = map[string]*CachedSearch{
		"call_flights_001": {
			Response:    []byte(`{"best_flights":[{"airline":"Iberia"}],"results_state":"complete"}`),
			Destination: "BCN→MAD",
			Type:        "flights",
		},
	}

	if err := store.Save(ctx, loaded); err != nil {
		t.Fatalf("Save (second POST) failed: %v", err)
	}

	// 4. GET again: Verify messages were appended + search cache persisted
	loaded2, err := store.Load(ctx, convID)
	if err != nil {
		t.Fatalf("Load (second GET) failed: %v", err)
	}
	if len(loaded2.Messages) != 2 {
		t.Errorf("Messages len = %d, want 2", len(loaded2.Messages))
	}
	if loaded2.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", loaded2.TurnCount)
	}
	if len(loaded2.SearchCache) != 1 {
		t.Errorf("SearchCache len = %d, want 1", len(loaded2.SearchCache))
	}

	// 5. DELETE
	if err := store.Delete(ctx, convID, "user-f5"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 6. GET after DELETE → 404 (nil)
	loaded3, err := store.Load(ctx, convID)
	if err != nil {
		t.Fatalf("Load after delete failed: %v", err)
	}
	if loaded3 != nil {
		t.Errorf("conversation should be nil after DELETE, got: %+v", loaded3)
	}
}

// =============================================================================
// Save nil / empty ID
// =============================================================================

func TestConversationStore_Save_NilConversation(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)

	err := store.Save(ctx, nil)
	if err == nil {
		t.Error("expected error for nil conversation, got nil")
	}
}

func TestConversationStore_Save_EmptyID(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	store := NewConvStore(rdb)

	err := store.Save(ctx, &Conversation{ID: ""})
	if err == nil {
		t.Error("expected error for empty ID, got nil")
	}
}
