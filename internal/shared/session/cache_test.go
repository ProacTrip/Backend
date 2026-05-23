package session

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// setupSessionTest crea un cliente redis respaldado por miniredis para testing.
func setupSessionTest(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

func TestSetAndGetSession(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	data := &SessionData{
		Permissions:   "users:read,roles:read",
		Status:        "active",
		TokenVersion:  "1",
		SchemaVersion: "1",
	}

	// Set
	if err := SetSession(ctx, rdb, "test-sid-1", data, SessionTTL); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	// Get
	got, err := GetSession(ctx, rdb, "test-sid-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got == nil {
		t.Fatal("GetSession returned nil after SetSession")
	}
	if got.Permissions != data.Permissions {
		t.Errorf("Permissions = %q, want %q", got.Permissions, data.Permissions)
	}
	if got.Status != data.Status {
		t.Errorf("Status = %q, want %q", got.Status, data.Status)
	}
	if got.TokenVersion != data.TokenVersion {
		t.Errorf("TokenVersion = %q, want %q", got.TokenVersion, data.TokenVersion)
	}
}

func TestGetSessionCacheMiss(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	got, err := GetSession(ctx, rdb, "non-existent")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil on cache miss, got %+v", got)
	}
}

func TestSetSessionNilData(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	// Nil data should be a no-op
	if err := SetSession(ctx, rdb, "test-sid-2", nil, SessionTTL); err != nil {
		t.Fatalf("SetSession with nil data should not error: %v", err)
	}
}

func TestInvalidateSession(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	data := &SessionData{
		Permissions: "users:read",
		Status:      "active",
	}

	// Set and verify
	if err := SetSession(ctx, rdb, "test-sid-3", data, SessionTTL); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	// Invalidate
	if err := InvalidateSession(ctx, rdb, "test-sid-3"); err != nil {
		t.Fatalf("InvalidateSession: %v", err)
	}

	// Verify deleted
	got, err := GetSession(ctx, rdb, "test-sid-3")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after invalidation, got %+v", got)
	}
}

func TestInvalidateSessionIdempotent(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	// Invalidating a non-existent session should not error
	if err := InvalidateSession(ctx, rdb, "non-existent"); err != nil {
		t.Fatalf("InvalidateSession should be idempotent: %v", err)
	}
}

func TestInvalidateSession_ByUserID(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	userA := "user-a-123"
	userB := "user-b-456"

	// Create session for user A (should be deleted)
	data := &SessionData{UserID: userA, Status: "active", TokenVersion: "1"}
	if err := SetSession(ctx, rdb, userA, data, SessionTTL); err != nil {
		t.Fatalf("SetSession userA: %v", err)
	}

	// Create session for user B (should survive)
	dataB := &SessionData{UserID: userB, Status: "active", TokenVersion: "1"}
	if err := SetSession(ctx, rdb, userB, dataB, SessionTTL); err != nil {
		t.Fatalf("SetSession userB: %v", err)
	}

	// Invalidate only user A (single-key delete)
	if err := InvalidateSession(ctx, rdb, userA); err != nil {
		t.Fatalf("InvalidateSession: %v", err)
	}

	// Verify user A session deleted
	gotA, err := GetSession(ctx, rdb, userA)
	if err != nil {
		t.Fatalf("GetSession userA: %v", err)
	}
	if gotA != nil {
		t.Errorf("expected nil after InvalidateSession for user A, got %+v", gotA)
	}

	// Verify user B session intact
	gotB, err := GetSession(ctx, rdb, userB)
	if err != nil {
		t.Fatalf("GetSession userB: %v", err)
	}
	if gotB == nil {
		t.Errorf("user B session should NOT have been deleted")
	}
}

func TestGetOrSetSessionCacheHit(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	data := &SessionData{
		Permissions: "users:read",
		Status:      "active",
		TokenVersion: "2",
	}

	// Populate cache
	if err := SetSession(ctx, rdb, "test-sid-4", data, SessionTTL); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	// GetOrSet should return from cache, never calling fn
	callCount := 0
	fn := func() (*SessionData, error) {
		callCount++
		return &SessionData{Status: "from-db"}, nil
	}

	got, err := GetOrSetSession(ctx, rdb, "test-sid-4", SessionTTL, fn)
	if err != nil {
		t.Fatalf("GetOrSetSession: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil data")
	}
	if got.TokenVersion != "2" {
		t.Errorf("TokenVersion = %q, want %q", got.TokenVersion, "2")
	}
	if callCount != 0 {
		t.Errorf("fn called %d times on cache hit, expected 0", callCount)
	}
}

func TestGetOrSetSessionCacheMiss(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	// Key does not exist
	fn := func() (*SessionData, error) {
		return &SessionData{
			Permissions:  "roles:read",
			Status:       "active",
			TokenVersion: "1",
		}, nil
	}

	got, err := GetOrSetSession(ctx, rdb, "test-sid-5", SessionTTL, fn)
	if err != nil {
		t.Fatalf("GetOrSetSession: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil data from fn")
	}
	if got.Permissions != "roles:read" {
		t.Errorf("Permissions = %q, want %q", got.Permissions, "roles:read")
	}

	// Now verify it's cached
	cached, err := GetSession(ctx, rdb, "test-sid-5")
	if err != nil {
		t.Fatalf("GetSession after GetOrSet: %v", err)
	}
	if cached == nil {
		t.Fatal("expected data cached after GetOrSet miss")
	}
}

func TestSetSessionDefaultSchemaVersion(t *testing.T) {
	rdb, _ := setupSessionTest(t)
	ctx := t.Context()

	// SchemaVersion empty → should default to "1"
	data := &SessionData{
		Permissions:  "users:read",
		Status:       "active",
		TokenVersion: "1",
		// SchemaVersion intentionally left empty
	}

	if err := SetSession(ctx, rdb, "test-sid-6", data, SessionTTL); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	got, err := GetSession(ctx, rdb, "test-sid-6")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q (default)", got.SchemaVersion, "1")
	}
}

func TestSessionKeyFormat(t *testing.T) {
	// Verify hashtag format for DragonflyDB shard affinity
	key := keyForSession("abc123")
	expected := "{auth}:session:abc123"
	if key != expected {
		t.Errorf("key = %q, want %q", key, expected)
	}
}
