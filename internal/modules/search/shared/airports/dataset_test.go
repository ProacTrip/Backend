package airports

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// TDD RED Phase — test file written BEFORE implementation of dataset.go
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

// =============================================================================
// Exact match tests (Tier 1)
// =============================================================================

func TestResolveIATA_ExactIATACode(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	entry, err := ResolveIATA(ctx, rdb, "MAD")
	if err != nil {
		t.Fatalf("ResolveIATA(MAD) returned error: %v", err)
	}
	if entry == nil {
		t.Fatal("ResolveIATA(MAD) returned nil entry")
	}
	if entry.IATA != "MAD" {
		t.Errorf("IATA = %q, want MAD", entry.IATA)
	}
	if entry.City != "Madrid" {
		t.Errorf("City = %q, want Madrid", entry.City)
	}
	if entry.Country != "España" {
		t.Errorf("Country = %q, want España", entry.Country)
	}
}

func TestResolveIATA_ExactCityName(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	entry, err := ResolveIATA(ctx, rdb, "Madrid")
	if err != nil {
		t.Fatalf("ResolveIATA(Madrid) returned error: %v", err)
	}
	if entry.IATA != "MAD" {
		t.Errorf("IATA = %q, want MAD", entry.IATA)
	}
}

func TestResolveIATA_ExactAliasMatch(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	entry, err := ResolveIATA(ctx, rdb, "Madrid-Barajas")
	if err != nil {
		t.Fatalf("ResolveIATA(Madrid-Barajas) returned error: %v", err)
	}
	if entry.IATA != "MAD" {
		t.Errorf("IATA = %q, want MAD", entry.IATA)
	}
}

// =============================================================================
// Fuzzy match tests (Tier 2)
// =============================================================================

func TestResolveIATA_FuzzyTypoCorrection(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	// "mdrid" should fuzzy-match "Madrid"
	entry, err := ResolveIATA(ctx, rdb, "mdrid")
	if err != nil {
		t.Fatalf("ResolveIATA(mdrid) returned error: %v", err)
	}
	if entry == nil {
		t.Fatal("ResolveIATA(mdrid) returned nil entry")
	}
	if entry.IATA != "MAD" {
		t.Errorf("IATA = %q, want MAD (fuzzy match for 'mdrid')", entry.IATA)
	}
}

func TestResolveIATA_FuzzyDifferentCase(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	entry, err := ResolveIATA(ctx, rdb, "buenos aires")
	if err != nil {
		t.Fatalf("ResolveIATA(buenos aires) returned error: %v", err)
	}
	if entry.IATA != "EZE" {
		t.Errorf("IATA = %q, want EZE", entry.IATA)
	}
}

// =============================================================================
// Not found tests (Tier 3 — AI fallback)
// =============================================================================

func TestResolveIATA_UnknownCity(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	entry, err := ResolveIATA(ctx, rdb, "zzzunknowncity")
	if !errors.Is(err, ErrIATANotFound) {
		t.Errorf("expected ErrIATANotFound, got: %v (entry: %v)", err, entry)
	}
	if entry != nil {
		t.Error("expected nil entry for unknown city")
	}
}

// =============================================================================
// Case insensitivity
// =============================================================================

func TestResolveIATA_CaseInsensitive(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	tests := []struct {
		query string
		want  string
	}{
		{"mad", "MAD"},
		{"LONDRES", "LHR"},
		{"nEw YoRk", "JFK"},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			entry, err := ResolveIATA(ctx, rdb, tc.query)
			if err != nil {
				t.Fatalf("ResolveIATA(%q) returned error: %v", tc.query, err)
			}
			if entry.IATA != tc.want {
				t.Errorf("ResolveIATA(%q).IATA = %q, want %q", tc.query, entry.IATA, tc.want)
			}
		})
	}
}

// =============================================================================
// AirportEntry structure validation
// =============================================================================

func TestAirportEntry_Fields(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	entry, err := ResolveIATA(ctx, rdb, "EZE")
	if err != nil {
		t.Fatalf("ResolveIATA(EZE) returned error: %v", err)
	}

	if entry.CountryCode != "AR" {
		t.Errorf("CountryCode = %q, want AR", entry.CountryCode)
	}
	if len(entry.Aliases) == 0 {
		t.Error("EZE should have aliases")
	}
}

// =============================================================================
// Top airports smoke check — ensure dataset is populated
// =============================================================================

func TestDataset_NotEmpty(t *testing.T) {
	ctx := t.Context()
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	// Check a random sample of major airports — if any fails, dataset is incomplete
	codes := []string{"MAD", "BCN", "EZE", "LHR", "JFK", "CDG", "DXB", "NRT", "SYD", "GRU", "MEX", "BOG", "SCL"}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			entry, err := ResolveIATA(ctx, rdb, code)
			if err != nil {
				t.Fatalf("ResolveIATA(%s) returned error: %v", code, err)
			}
			if entry == nil {
				t.Fatalf("ResolveIATA(%s) returned nil (dataset likely missing this airport)", code)
			}
			if entry.IATA != code {
				t.Errorf("ResolveIATA(%s).IATA = %q, want %s", code, entry.IATA, code)
			}
		})
	}
}

// =============================================================================
// nil rdb doesn't crash (AI fallback still returns ErrIATANotFound)
// =============================================================================

func TestResolveIATA_NilRedisClient_ReturnsError(t *testing.T) {
	ctx := t.Context()

	entry, err := ResolveIATA(ctx, nil, "zzzunknown")

	if !errors.Is(err, ErrIATANotFound) {
		t.Errorf("expected ErrIATANotFound, got: %v", err)
	}
	if entry != nil {
		t.Error("expected nil entry")
	}
}

// =============================================================================
// Compile-time check that ErrIATANotFound is a proper sentinel error
// =============================================================================

func TestErrIATANotFound_IsDistinct(t *testing.T) {
	if errors.Is(ErrIATANotFound, context.Canceled) {
		t.Error("ErrIATANotFound should not match context.Canceled")
	}
	if ErrIATANotFound.Error() == "" {
		t.Error("ErrIATANotFound should have a non-empty error message")
	}
}
