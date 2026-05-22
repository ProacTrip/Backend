package shared

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Tests for ResolveSearchDefaults — 3-tier per-param resolution
// =============================================================================

func setupDefaultsTest(t *testing.T) (*redis.Client, *miniredis.Miniredis, SearchDefaultConfig) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	cfg := SearchDefaultConfig{
		Currency: "EUR",
		Language: "es",
	}
	return rdb, mr, cfg
}

// ===================== Tier 1: Explicit params win per-param =====================

func TestResolveSearchDefaults_Tier1_ExplicitWins(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"user-123",    // userID (should be ignored because all explicit params present)
		"192.168.1.1", // clientIP
		new("US"),     // explicitGL
		new("en"),     // explicitHL
		new("USD"),    // explicitCurrency
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

func TestResolveSearchDefaults_Tier1_SingleExplicitWithConfigFallback(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	// Only currency is explicit — GL and HL fall through to config defaults
	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"user-123",
		"192.168.1.1",
		nil,
		nil,
		new("GBP"),
		cfg,
	)

	if gl != "" {
		t.Errorf("gl = %q, want %q (GL no longer resolved by ResolveSearchDefaults — Phase 2 ai-discovery-rewrite)", gl, "")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q (config default)", hl, "es")
	}
	if currency != "GBP" {
		t.Errorf("currency = %q, want %q", currency, "GBP")
	}
}

// ===================== Tier 2: Authenticated profile prefs (HL and Currency) =====================

func TestResolveSearchDefaults_Tier2_ProfilePrefs(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()
	userID := "550e8400-e29b-41d4-a716-446655440000"

	// Populate profile prefs cache
	key := "user:prefs:" + userID
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

	// GL comes from config default (country_code is not in the 3-tier for GL)
	if gl != "" {
		t.Errorf("gl = %q, want %q (GL no longer resolved by ResolveSearchDefaults — Phase 2 ai-discovery-rewrite)", gl, "")
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

	if gl != "" {
		t.Errorf("gl = %q, want %q (GL no longer resolved by ResolveSearchDefaults — Phase 2 ai-discovery-rewrite)", gl, "")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q (config default)", hl, "es")
	}
	if currency != "EUR" {
		t.Errorf("currency = %q, want %q (config default)", currency, "EUR")
	}
}

// ===================== Tier 3: Config fallback =====================

func TestResolveSearchDefaults_Tier3_ConfigFallback(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"", "", // anonymous, no IP
		nil, nil, nil,
		cfg,
	)

	if gl != "" {
		t.Errorf("gl = %q, want %q (GL no longer resolved by ResolveSearchDefaults — Phase 2 ai-discovery-rewrite)", gl, "")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q", hl, "es")
	}
	if currency != "EUR" {
		t.Errorf("currency = %q, want %q", currency, "EUR")
	}
}

// ===================== Per-param: HL and Currency from profile, GL from config =====================

func TestResolveSearchDefaults_PerParam_ExplicitHLOverridesProfile(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()
	userID := "550e8400-e29b-41d4-a716-446655440001"

	// Populate profile prefs (Spanish/Argentina)
	rdb.HSet(ctx, "user:prefs:"+userID, map[string]interface{}{
		"currency":     "ARS",
		"language":     "es",
		"country_code": "AR",
	})

	// Explicit HL=en should override profile language=es, but currency falls to profile
	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		userID, "",
		nil,      // no explicit GL
		new("en"), // explicit HL
		nil,      // no explicit currency
		cfg,
	)

	if gl != "" {
		t.Errorf("gl = %q, want %q (GL no longer resolved by ResolveSearchDefaults — Phase 2 ai-discovery-rewrite)", gl, "")
	}
	if hl != "en" {
		t.Errorf("hl = %q, want %q (explicit wins)", hl, "en")
	}
	if currency != "ARS" {
		t.Errorf("currency = %q, want %q (profile prefs fallback)", currency, "ARS")
	}
}

// ===================== Dragonfly down — graceful fallback to config =====================

// TestResolveSearchDefaults_DragonflyDown_FallsToConfig verifies that when
// the Redis/Dragonfly connection is down (closed), ResolveSearchDefaults does
// NOT panic and gracefully falls through to Tier 3 config defaults.
func TestResolveSearchDefaults_DragonflyDown_FallsToConfig(t *testing.T) {
	rdb, mr, cfg := setupDefaultsTest(t)
	ctx := t.Context()

	// Close the Redis connection to simulate Dragonfly being down.
	if err := rdb.Close(); err != nil {
		t.Fatalf("failed to close redis: %v", err)
	}
	mr.Close()

	// Should NOT panic — must return config defaults gracefully
	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		"user-123", "1.2.3.4",
		nil, nil, nil,
		cfg,
	)

	if gl != "" {
		t.Errorf("gl = %q, want %q (GL no longer resolved by ResolveSearchDefaults — Phase 2 ai-discovery-rewrite)", gl, "")
	}
	if hl != "es" {
		t.Errorf("hl = %q, want %q (config fallback)", hl, "es")
	}
	if currency != "EUR" {
		t.Errorf("currency = %q, want %q (config fallback)", currency, "EUR")
	}
}

// ===================== Tier 1 beats all, even with cache populated =====================

func TestResolveSearchDefaults_Tier1BeatsAll(t *testing.T) {
	rdb, _, cfg := setupDefaultsTest(t)
	ctx := t.Context()
	userID := "550e8400-e29b-41d4-a716-446655440002"

	// Populate profile prefs
	rdb.HSet(ctx, "user:prefs:"+userID, map[string]interface{}{
		"currency":     "ARS",
		"language":     "es",
		"country_code": "AR",
	})

	// Tier 1: explicit always wins
	gl, hl, currency := ResolveSearchDefaults(ctx, rdb,
		userID, "",
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
