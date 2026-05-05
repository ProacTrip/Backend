package shared

import (
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Tests for ResolveSearchDefaults — 4-tier priority verification
// =============================================================================

func setupDefaultsTest(t *testing.T) (*redis.Client, *miniredis.Miniredis, SearchDefaultConfig) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	cfg := SearchDefaultConfig{
		Currency:    "EUR",
		Language:    "es",
		CountryCode: "AR",
	}
	return rdb, mr, cfg
}

// ===================== Tier 1: Explicit params always win =====================

func TestResolveSearchDefaults_Tier1_ExplicitWins(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"user-123",          // userID (should be ignored because explicit params present)
		"192.168.1.1",       // clientIP
		new("US"),            // explicitGL
		new("en"),            // explicitHL
		new("USD"),           // explicitCurrency
		cfg,
	)

	if gl != "US" {
		t.Errorf("gl = %q, want %q", gl, "US")
	}
	if hl != "en" {
		t.Errorf("hl = %q, want %q", hl, "en")
	}
	if currency != "USD" {
		t.Errorf("currency = %q, want %q", currency, "USD")
	}
}

func TestResolveSearchDefaults_Tier1_SingleExplicitWins(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	// Only currency is explicit — still tier 1 wins (override completely)
	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"user-123",
		"192.168.1.1",
		nil,
		nil,
		new("GBP"),
		cfg,
	)

	if gl != "" {
		t.Errorf("gl = %q, want empty (nil explicit)", gl)
	}
	if hl != "" {
		t.Errorf("hl = %q, want empty (nil explicit)", hl)
	}
	if currency != "GBP" {
		t.Errorf("currency = %q, want %q", currency, "GBP")
	}
}

// ===================== Tier 2: Authenticated profile prefs =====================

func TestResolveSearchDefaults_Tier2_ProfilePrefs(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()
	userID := "550e8400-e29b-41d4-a716-446655440000"

	// Populate profile prefs cache
	key := "profile:" + userID + ":prefs"
	fields := map[string]interface{}{
		"currency":     "BRL",
		"language":     "pt",
		"country_code": "BR",
		"timezone":     "America/Sao_Paulo",
	}
	if err := rdb.HSet(ctx, key, fields).Err(); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		userID,
		"",
		nil, // no explicit
		nil,
		nil,
		cfg,
	)

	if gl != "BR" {
		t.Errorf("gl = %q, want %q (from country_code)", gl, "BR")
	}
	if hl != "pt" {
		t.Errorf("hl = %q, want %q (from language)", hl, "pt")
	}
	if currency != "BRL" {
		t.Errorf("currency = %q, want %q", currency, "BRL")
	}
}

func TestResolveSearchDefaults_Tier2_ProfilePrefsMiss(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()
	userID := "nonexistent-user"

	// No profile prefs in cache — should fall through to config defaults
	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		userID,
		"", // no IP
		nil, nil, nil,
		cfg,
	)

	if gl != "AR" {
		t.Errorf("gl = %q, want %q (config default)", gl, "AR")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q (config default)", hl, "es")
	}
	if currency != "EUR" {
		t.Errorf("currency = %q, want %q (config default)", currency, "EUR")
	}
}

// ===================== Tier 3: Anonymous env cache =====================

func TestResolveSearchDefaults_Tier3_EnvCache(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()
	ip := "8.8.8.8"

	// Populate env:{ip} cache (same format as environment usecase stores)
	envData := envCacheEntry{}
	envData.Location.CountryCode = "JP"
	envData.Location.Language = "ja"
	envData.Location.Currency = "JPY"
	raw, err := json.Marshal(envData)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := rdb.Set(ctx, "env:"+ip, string(raw), 0).Err(); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"",   // no userID → skip tier 2
		ip,   // has IP → hit tier 3
		nil, nil, nil,
		cfg,
	)

	if gl != "JP" {
		t.Errorf("gl = %q, want %q", gl, "JP")
	}
	if hl != "ja" {
		t.Errorf("hl = %q, want %q", hl, "ja")
	}
	if currency != "JPY" {
		t.Errorf("currency = %q, want %q", currency, "JPY")
	}
}

func TestResolveSearchDefaults_Tier3_EnvCacheMiss(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"",
		"10.0.0.1", // no env cache for this IP
		nil, nil, nil,
		cfg,
	)

	if gl != "AR" {
		t.Errorf("gl = %q, want %q (config fallback)", gl, "AR")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q (config fallback)", hl, "es")
	}
	if currency != "EUR" {
		t.Errorf("currency = %q, want %q (config fallback)", currency, "EUR")
	}
}

// ===================== Tier 4: Config fallback =====================

func TestResolveSearchDefaults_Tier4_ConfigFallback(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"", "", // no user, no IP
		nil, nil, nil,
		cfg,
	)

	if gl != "AR" {
		t.Errorf("gl = %q, want %q", gl, "AR")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q", hl, "es")
	}
	if currency != "EUR" {
		t.Errorf("currency = %q, want %q", currency, "EUR")
	}
}

// ===================== Tier priority: 2 beats 3 =====================

func TestResolveSearchDefaults_Tier2BeatsTier3(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()
	userID := "550e8400-e29b-41d4-a716-446655440001"
	ip := "1.1.1.1"

	// Set up Tier 2 (profile prefs)
	profileKey := "profile:" + userID + ":prefs"
	rdb.HSet(ctx, profileKey, map[string]interface{}{
		"currency":     "ARS",
		"language":     "es",
		"country_code": "AR",
		"timezone":     "America/Argentina/Buenos_Aires",
	})

	// Set up Tier 3 (env cache — should be ignored because Tier 2 is found)
	envData := envCacheEntry{}
	envData.Location.CountryCode = "US"
	envData.Location.Language = "en"
	envData.Location.Currency = "USD"
	raw, _ := json.Marshal(envData)
	rdb.Set(ctx, "env:"+ip, string(raw), 0)

	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		userID, ip,
		nil, nil, nil,
		cfg,
	)

	// Tier 2 should win over Tier 3
	if gl != "AR" {
		t.Errorf("gl = %q, want %q (profile prefs should beat env cache)", gl, "AR")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q (profile prefs should beat env cache)", hl, "es")
	}
	if currency != "ARS" {
		t.Errorf("currency = %q, want %q (profile prefs should beat env cache)", currency, "ARS")
	}
}

// ===================== Dragonfly down — graceful fallback to config =====================

// TestResolveSearchDefaults_DragonflyDown_FallsToConfig verifies that when
// the Redis/Dragonfly connection is down (closed), ResolveSearchDefaults does
// NOT panic and gracefully falls through to Tier 4 config defaults.
// Both Tier 2 (profile prefs) and Tier 3 (env cache) hit connection errors
// and are handled by the error branches in the code.
func TestResolveSearchDefaults_DragonflyDown_FallsToConfig(t *testing.T) {
	rdb, mr, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	// Close the Redis connection to simulate Dragonfly being down.
	// Subsequent HGetAll (Tier 2) and Get (Tier 3) calls will return errors.
	if err := rdb.Close(); err != nil {
		t.Fatalf("failed to close redis: %v", err)
	}
	// Also shut down miniredis to ensure no reconnection
	mr.Close()

	// Should NOT panic — must return config defaults gracefully
	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"user-123", "1.2.3.4",
		nil, nil, nil,
		cfg,
	)

	if gl != "AR" {
		t.Errorf("gl = %q, want %q (Tier 4 config fallback)", gl, "AR")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q (Tier 4 config fallback)", hl, "es")
	}
	if currency != "EUR" {
		t.Errorf("currency = %q, want %q (Tier 4 config fallback)", currency, "EUR")
	}
}

// ===================== Tier 1 beats all, even with cache populated =====================

func TestResolveSearchDefaults_Tier1BeatsAll(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()
	userID := "550e8400-e29b-41d4-a716-446655440002"
	ip := "2.2.2.2"

	// Populate Tier 2
	rdb.HSet(ctx, "profile:"+userID+":prefs", map[string]interface{}{
		"currency":     "ARS",
		"language":     "es",
		"country_code": "AR",
	})

	// Populate Tier 3
	envData := envCacheEntry{}
	envData.Location.CountryCode = "US"
	envData.Location.Language = "en"
	envData.Location.Currency = "USD"
	raw, _ := json.Marshal(envData)
	rdb.Set(ctx, "env:"+ip, string(raw), 0)

	// Tier 1: explicit always wins
	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		userID, ip,
		new("GB"), new("en"), new("GBP"),
		cfg,
	)

	if gl != "GB" {
		t.Errorf("gl = %q, want %q", gl, "GB")
	}
	if hl != "en" {
		t.Errorf("hl = %q, want %q", hl, "en")
	}
	if currency != "GBP" {
		t.Errorf("currency = %q, want %q", currency, "GBP")
	}
}
