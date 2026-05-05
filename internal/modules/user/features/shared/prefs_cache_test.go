package shared

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Tests for GetProfilePrefs cache helper
// =============================================================================

func setupProfileCache(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

func TestGetProfilePrefs_CacheHit(t *testing.T) {
	rdb, _ := setupProfileCache(t)
	ctx := t.Context()
	userID := "550e8400-e29b-41d4-a716-446655440000"
	key := "profile:" + userID + ":prefs"

	// Pre-populate the cache as if a prior call stored it
	fields := map[string]interface{}{
		"currency":     "ARS",
		"language":     "es",
		"country_code": "AR",
		"timezone":     "America/Argentina/Buenos_Aires",
	}
	if err := rdb.HSet(ctx, key, fields).Err(); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}
	if err := rdb.Expire(ctx, key, 30*time.Minute).Err(); err != nil {
		t.Fatalf("Expire failed: %v", err)
	}

	currency, language, countryCode, timezone, found, err := GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for pre-populated cache, got false")
	}
	if currency != "ARS" {
		t.Errorf("currency = %q, want %q", currency, "ARS")
	}
	if language != "es" {
		t.Errorf("language = %q, want %q", language, "es")
	}
	if countryCode != "AR" {
		t.Errorf("countryCode = %q, want %q", countryCode, "AR")
	}
	if timezone != "America/Argentina/Buenos_Aires" {
		t.Errorf("timezone = %q, want %q", timezone, "America/Argentina/Buenos_Aires")
	}
}

func TestGetProfilePrefs_CacheMiss(t *testing.T) {
	rdb, _ := setupProfileCache(t)
	ctx := t.Context()
	userID := "660e8400-e29b-41d4-a716-446655440001"

	currency, language, countryCode, timezone, found, err := GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if found {
		t.Fatal("expected found=false for non-existent cache key, got true")
	}
	// On miss, all values should be zero
	if currency != "" {
		t.Errorf("currency = %q, want empty on miss", currency)
	}
	if language != "" {
		t.Errorf("language = %q, want empty on miss", language)
	}
	if countryCode != "" {
		t.Errorf("countryCode = %q, want empty on miss", countryCode)
	}
	if timezone != "" {
		t.Errorf("timezone = %q, want empty on miss", timezone)
	}
}

func TestGetProfilePrefs_CacheKeyFormat(t *testing.T) {
	rdb, _ := setupProfileCache(t)
	ctx := t.Context()
	userID := "770e8400-e29b-41d4-a716-446655440002"
	expectedKey := "profile:" + userID + ":prefs"

	// Populate with exactly the expected key format
	fields := map[string]interface{}{
		"currency": "USD",
	}
	if err := rdb.HSet(ctx, expectedKey, fields).Err(); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	// The function must use the exact key format "profile:{userID}:prefs"
	currency, _, _, _, found, err := GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true — key format must be profile:{userID}:prefs")
	}
	if currency != "USD" {
		t.Errorf("currency = %q, want %q", currency, "USD")
	}
}

func TestGetProfilePrefs_PartialHashMap(t *testing.T) {
	rdb, _ := setupProfileCache(t)
	ctx := t.Context()
	userID := "880e8400-e29b-41d4-a716-446655440003"
	key := "profile:" + userID + ":prefs"

	// Only set some fields — the others should return empty
	fields := map[string]interface{}{
		"language": "pt",
	}
	if err := rdb.HSet(ctx, key, fields).Err(); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	currency, language, countryCode, timezone, found, err := GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true when hash exists with partial fields")
	}
	if currency != "" {
		t.Errorf("currency = %q, want empty (field not set)", currency)
	}
	if language != "pt" {
		t.Errorf("language = %q, want %q", language, "pt")
	}
	if countryCode != "" {
		t.Errorf("countryCode = %q, want empty (field not set)", countryCode)
	}
	if timezone != "" {
		t.Errorf("timezone = %q, want empty (field not set)", timezone)
	}
}

// =============================================================================
// Tests for SetProfilePrefs
// =============================================================================

func TestSetProfilePrefs_WritesAllFields(t *testing.T) {
	rdb, _ := setupProfileCache(t)
	ctx := t.Context()
	userID := "990e8400-e29b-41d4-a716-446655440004"

	err := SetProfilePrefs(ctx, rdb, userID, "ARS", "es", "AR", "America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("SetProfilePrefs error: %v", err)
	}

	// Read back via GetProfilePrefs
	currency, language, countryCode, timezone, found, err := GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true after SetProfilePrefs")
	}
	if currency != "ARS" {
		t.Errorf("currency = %q, want %q", currency, "ARS")
	}
	if language != "es" {
		t.Errorf("language = %q, want %q", language, "es")
	}
	if countryCode != "AR" {
		t.Errorf("countryCode = %q, want %q", countryCode, "AR")
	}
	if timezone != "America/Argentina/Buenos_Aires" {
		t.Errorf("timezone = %q, want %q", timezone, "America/Argentina/Buenos_Aires")
	}
}

func TestSetProfilePrefs_EmptyFieldsSkipped(t *testing.T) {
	rdb, _ := setupProfileCache(t)
	ctx := t.Context()
	userID := "aa0e8400-e29b-41d4-a716-446655440005"

	// Only set some fields — others empty
	err := SetProfilePrefs(ctx, rdb, userID, "BRL", "", "", "")
	if err != nil {
		t.Fatalf("SetProfilePrefs error: %v", err)
	}

	currency, language, countryCode, timezone, found, err := GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true (hash exists with at least one field)")
	}
	if currency != "BRL" {
		t.Errorf("currency = %q, want %q", currency, "BRL")
	}
	if language != "" {
		t.Errorf("language = %q, want empty (not set)", language)
	}
	if countryCode != "" {
		t.Errorf("countryCode = %q, want empty (not set)", countryCode)
	}
	if timezone != "" {
		t.Errorf("timezone = %q, want empty (not set)", timezone)
	}
}

func TestSetProfilePrefs_AllEmpty(t *testing.T) {
	rdb, _ := setupProfileCache(t)
	ctx := t.Context()
	userID := "bb0e8400-e29b-41d4-a716-446655440006"

	// All fields empty — no-op
	err := SetProfilePrefs(ctx, rdb, userID, "", "", "", "")
	if err != nil {
		t.Fatalf("SetProfilePrefs error: %v", err)
	}

	// Should be cache miss (nothing was stored)
	_, _, _, _, found, err := GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if found {
		t.Fatal("expected found=false when all fields were empty")
	}
}
