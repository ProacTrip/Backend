// Tests del Hub: entrega local, PublishAndBridge, publishLocal.
package sse

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// newTestRedis creates a miniredis-backed redis.Client for testing.
func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

// =============================================================================
// Tests de publishLocal
// =============================================================================

func TestPublish_Local(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()
	ch := hub.Subscribe(userID)

	hub.Publish(userID, Event{Type: "test.event", Data: "hello"})

	select {
	case evt := <-ch:
		if evt.Type != "test.event" {
			t.Errorf("event type = %q, want %q", evt.Type, "test.event")
		}
	default:
		t.Fatal("expected event on channel, got nothing")
	}
}

func TestPublish_NoSubscribers(t *testing.T) {
	hub := NewHub()
	// No subscribers — should not panic
	hub.Publish(uuid.New(), Event{Type: "test.event", Data: "hello"})
}

// =============================================================================
// Tests de PublishAndBridge
// =============================================================================

func TestPublishAndBridge_LocalDelivery(t *testing.T) {
	hub := NewHub()
	rdb, _ := newTestRedis(t)
	userID := uuid.New()
	ch := hub.Subscribe(userID)

	hub.PublishAndBridge(t.Context(), rdb, userID, Event{Type: "test.event", Data: "hello"})

	select {
	case evt := <-ch:
		if evt.Type != "test.event" {
			t.Errorf("event type = %q, want %q", evt.Type, "test.event")
		}
	default:
		t.Fatal("expected event on channel, got nothing")
	}
}

func TestPublishAndBridge_RdbPublish(t *testing.T) {
	hub := NewHub()
	rdb, _ := newTestRedis(t)
	userID := uuid.New()
	ctx := t.Context()

	// Subscribe a listener to verify the bridge message is published
	pubsub := rdb.Subscribe(ctx, "{sse}:events")
	defer pubsub.Close()

	// Give miniredis time to establish subscription
	time.Sleep(10 * time.Millisecond)
	ch := pubsub.Channel()

	hub.PublishAndBridge(ctx, rdb, userID, Event{Type: "test.event", Data: map[string]any{"key": "value"}})

	select {
	case msg := <-ch:
		var bm BridgeMessage
		if err := json.Unmarshal([]byte(msg.Payload), &bm); err != nil {
			t.Fatalf("failed to unmarshal bridge message: %v", err)
		}
		if bm.UserID != userID.String() {
			t.Errorf("UserID = %q, want %q", bm.UserID, userID.String())
		}
		if bm.EventType != "test.event" {
			t.Errorf("EventType = %q, want %q", bm.EventType, "test.event")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected Pub/Sub message, got nothing")
	}
}

func TestPublishAndBridge_NilRdb(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()
	ch := hub.Subscribe(userID)

	// Nil rdb should NOT panic — local delivery only
	hub.PublishAndBridge(t.Context(), nil, userID, Event{Type: "test.event", Data: "hello"})

	select {
	case evt := <-ch:
		if evt.Type != "test.event" {
			t.Errorf("event type = %q, want %q", evt.Type, "test.event")
		}
	default:
		t.Fatal("expected event on channel with nil rdb")
	}
}

func TestPublishAndBridge_NoSubscribers_NoPanic(t *testing.T) {
	hub := NewHub()
	rdb, _ := newTestRedis(t)

	// No subscribers — PublishAndBridge should not panic
	hub.PublishAndBridge(t.Context(), rdb, uuid.New(), Event{Type: "test.event", Data: "hello"})
}

func TestPublishAndBridge_PubSubErrorStillDeliversLocal(t *testing.T) {
	hub := NewHub()
	rdb, _ := newTestRedis(t)
	userID := uuid.New()
	ch := hub.Subscribe(userID)

	// Close rdb to trigger Publish error — local delivery should still work
	rdb.Close()

	hub.PublishAndBridge(t.Context(), rdb, userID, Event{Type: "test.event", Data: "hello"})

	select {
	case evt := <-ch:
		if evt.Type != "test.event" {
			t.Errorf("event type = %q, want %q", evt.Type, "test.event")
		}
	default:
		t.Fatal("expected local delivery even when Pub/Sub fails")
	}
}
