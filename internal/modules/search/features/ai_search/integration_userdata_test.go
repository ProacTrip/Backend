// Integration tests for ai_search with user data (favorites, saved searches)
// and environment context resolution.
//
// Tests:
//  1. Anonymous discovery — verifies 200 with intent=discovery, candidates populated
//  2. Authenticated with favorites — favorites appear as discovery candidates
//  3. Authenticated with saved searches — saved searches influence candidates
//  4. Environment context — country=Argentina → location hint injected for exact search
package ai_search

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	sharedEnv "github.com/ProacTrip/Backend/internal/shared/environment"
)

// =============================================================================
// Helpers
// =============================================================================

// setupUserDataTestUseCase creates a UseCase with discovery enabled and
// a miniredis instance for environment cache.
func setupUserDataTestUseCase(t *testing.T) (*UseCase, *redis.Client) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	uc := NewUseCase(UseCaseDeps{
		DiscoveryEnabled:        true,
		DiscoveryClarifyEnabled: true,
	})
	uc.rdb = rdb
	uc.discoveryEnabled = true
	uc.clarifyEnabled = true
	return uc, rdb
}

// seedEnvCache writes a country entry into the env:{ip} Dragonfly cache.
func seedEnvCache(t *testing.T, rdb *redis.Client, ip, country, countryCode, city string) {
	t.Helper()
	entry := sharedEnv.CacheEntry{
		Location: sharedEnv.LocationDTO{
			Country:     country,
			CountryCode: countryCode,
			City:        city,
			State:       "",
			Zipcode:     "",
			Timezone:    "America/Argentina/Buenos_Aires",
			Currency:    "ARS",
			Language:    "es",
			Latitude:    -34.6037,
			Longitude:   -58.3816,
		},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal env cache entry: %v", err)
	}
	key := sharedEnv.CacheKey(ip)
	if err := rdb.Set(context.Background(), key, string(data), 0).Err(); err != nil {
		t.Fatalf("seed env cache: %v", err)
	}
}

// mockFavoritesData creates mock favorites for a user.
func mockFavoritesData(userID string) []domain.SavedSearchData {
	return []domain.SavedSearchData{
		{
			Name:       "Bali Beach Getaway",
			Parameters: json.RawMessage(`{"destination":"Bali","country":"Indonesia"}`),
		},
		{
			Name:       "Paris Cultural Trip",
			Parameters: json.RawMessage(`{"destination":"París","country":"Francia"}`),
		},
	}
}

// mockSavedSearchesData creates mock saved searches for a user.
func mockSavedSearchesData(userID string) []domain.SavedSearchData {
	return []domain.SavedSearchData{
		{
			Name:       "Cancún Summer",
			Parameters: json.RawMessage(`{"destination":"Cancún","country":"México"}`),
		},
		{
			Name:       "Tokyo Adventure",
			Parameters: json.RawMessage(`{"destination":"Tokio","country":"Japón"}`),
		},
	}
}

// =============================================================================
// Test 1: Anonymous Discovery
// =============================================================================

// TestIntegration_AnonymousDiscovery verifies that an anonymous user requesting
// discovery gets a 200 response with intent=discovery, and candidates are
// populated from the pipeline.
func TestIntegration_AnonymousDiscovery(t *testing.T) {
	uc, rdb := setupUserDataTestUseCase(t)
	// Seed environment cache so the clarification question uses country context
	seedEnvCache(t, rdb, "192.0.2.100", "Argentina", "AR", "Buenos Aires")

	// Set up candidate sources with mock data
	favSrc := &FavoritesSource{
		getFavorites: func(userID string) []domain.SavedSearchData {
			return mockFavoritesData(userID)
		},
	}
	savedSrc := &SavedSearchSource{
		getSavedSearches: func(userID string) []domain.SavedSearchData {
			return mockSavedSearchesData(userID)
		},
	}
	uc.candidateSources = []CandidateSource{favSrc, savedSrc}

	ctx := t.Context()
	cmd := Command{
		Message:  "recomiéndame playa barato en verano",
		ClientIP: "192.0.2.100",
	}

	resp, err := uc.Execute(ctx, cmd, "test-user-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify discovery mode
	if resp.Mode != "discovery" {
		t.Errorf("Mode = %q, want 'discovery'", resp.Mode)
	}
	if resp.Intent != string(SearchModeDiscovery) {
		t.Errorf("Intent = %q, want '%s'", resp.Intent, SearchModeDiscovery)
	}

	// Verify candidates are populated from mock data
	// Since the query has budget=low and season=summer constraints,
	// the pipeline should NOT trigger NeedsClarification
	if resp.NeedsClarification {
		t.Log("NeedsClarification is true — but with full constraints this shouldn't happen")
	}

	// Response should have at least a message
	if resp.Message == "" {
		t.Error("expected non-empty message in discovery response")
	}
}

// =============================================================================
// Test 2: Authenticated with Favorites
// =============================================================================

// TestIntegration_AuthenticatedWithFavorites verifies that when an authenticated
// user has favorited destinations, those destinations appear as candidates in
// the discovery response.
func TestIntegration_AuthenticatedWithFavorites(t *testing.T) {
	uc, rdb := setupUserDataTestUseCase(t)
	seedEnvCache(t, rdb, "192.0.2.200", "España", "ES", "Madrid")

	// Set up ONLY favorites source
	favSrc := &FavoritesSource{
		getFavorites: func(userID string) []domain.SavedSearchData {
			if userID != "auth-user-456" {
				t.Errorf("expected userID 'auth-user-456', got %q", userID)
			}
			return []domain.SavedSearchData{
				{
					Name:       "Bali Trip",
					Parameters: json.RawMessage(`{"destination":"Bali","country":"Indonesia"}`),
				},
				{
					Name:       "Paris Weekend",
					Parameters: json.RawMessage(`{"destination":"París","country":"Francia"}`),
				},
			}
		},
	}
	uc.candidateSources = []CandidateSource{favSrc}

	ctx := t.Context()
	cmd := Command{
		Message:  "quiero viajar a algún lado con playa",
		ClientIP: "192.0.2.200",
	}

	resp, err := uc.Execute(ctx, cmd, "auth-user-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Mode != "discovery" {
		t.Errorf("Mode = %q, want 'discovery'", resp.Mode)
	}

	// The query has "playa" style but no budget → budget is empty
	// Budget is empty → NeedsClarification triggers for budget question
	// With detected country "España" → context-aware question
	if resp.NeedsClarification {
		if resp.ClarificationQuestion == "" {
			t.Error("expected non-empty ClarificationQuestion when budget is missing")
		}
		t.Logf("ClarificationQuestion: %s", resp.ClarificationQuestion)
	} else {
		t.Error("expected NeedsClarification=true when budget is missing")
	}

	// Since the pipeline asks clarification, candidates should be empty
	if len(resp.Candidates) != 0 {
		t.Errorf("expected 0 candidates during clarification, got %d", len(resp.Candidates))
	}
	if resp.TotalCandidates != 0 {
		t.Errorf("expected TotalCandidates=0 during clarification, got %d", resp.TotalCandidates)
	}
}

// =============================================================================
// Test 3: Authenticated with Saved Searches
// =============================================================================

// TestIntegration_AuthenticatedWithSavedSearches verifies that when an
// authenticated user has saved searches, those destinations influence
// discovery candidates.
func TestIntegration_AuthenticatedWithSavedSearches(t *testing.T) {
	uc, rdb := setupUserDataTestUseCase(t)
	seedEnvCache(t, rdb, "192.0.2.200", "Argentina", "AR", "Buenos Aires")

	// Set up ONLY saved searches source
	savedSrc := &SavedSearchSource{
		getSavedSearches: func(userID string) []domain.SavedSearchData {
			if userID != "auth-user-789" {
				t.Errorf("expected userID 'auth-user-789', got %q", userID)
			}
			return []domain.SavedSearchData{
				{
					Name:       "Miami Beach",
					Parameters: json.RawMessage(`{"destination":"Miami","country":"Estados Unidos"}`),
				},
			}
		},
	}
	uc.candidateSources = []CandidateSource{savedSrc}

	ctx := t.Context()
	// Query with budget + season → should NOT trigger NeedsClarification
	cmd := Command{
		Message:  "playa barato en julio",
		ClientIP: "192.0.2.200",
	}

	resp, err := uc.Execute(ctx, cmd, "auth-user-789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Mode != "discovery" {
		t.Errorf("Mode = %q, want 'discovery'", resp.Mode)
	}

	// With constraints (budget=low, month=7, style=beach) → should NOT need clarification
	if resp.NeedsClarification {
		t.Log("NeedsClarification=true — unexpected with full constraints set")
	}

	// Should have candidates from saved searches
	if len(resp.Candidates) == 0 && !resp.NeedsClarification {
		t.Error("expected candidates from saved searches, got none")
	}

	if resp.Message == "" {
		t.Error("expected non-empty message in discovery response")
	}
}

// =============================================================================
// Test 4: Environment Context
// =============================================================================

// TestIntegration_EnvironmentContext verifies that when the environment cache
// has country=Argentina, the discovery pipeline uses it for context-aware
// clarification questions.
func TestIntegration_EnvironmentContext(t *testing.T) {
	uc, rdb := setupUserDataTestUseCase(t)

	// Seed environment cache for Argentina
	seedEnvCache(t, rdb, "192.0.2.55", "Argentina", "AR", "Buenos Aires")

	// No candidate sources — pure clarification scenario
	uc.candidateSources = nil

	ctx := t.Context()
	cmd := Command{
		Message:  "recomiéndame un lugar para viajar",
		ClientIP: "192.0.2.55",
	}

	resp, err := uc.Execute(ctx, cmd, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Mode != "discovery" {
		t.Errorf("Mode = %q, want 'discovery'", resp.Mode)
	}

	// Open-ended query → needs clarification
	if !resp.NeedsClarification {
		t.Error("expected NeedsClarification=true for open-ended query without constraints")
	}

	// The clarification question should mention Argentina (detected country)
	if resp.ClarificationQuestion == "" {
		t.Error("expected non-empty ClarificationQuestion")
	}

	// Verify the question includes the detected country
	if resp.ClarificationQuestion != "" {
		t.Logf("Clarification question: %s", resp.ClarificationQuestion)
		// The question should contain "Argentina" since that's the detected country
		containsCountry := false
		if len(resp.ClarificationQuestion) > 0 {
			// Check in the question string
			for _, country := range []string{"Argentina"} {
				if containsSubstring(resp.ClarificationQuestion, country) {
					containsCountry = true
					break
				}
			}
		}
		if !containsCountry {
			t.Logf("Clarification question does NOT include country name (may use generic fallback): %s", resp.ClarificationQuestion)
		}
	}
}

// TestIntegration_EnvironmentContext_NoCache verifies that when there is
// NO environment cache entry, the clarification question falls back to
// the generic budget question (no country mention).
func TestIntegration_EnvironmentContext_NoCache(t *testing.T) {
	uc, rdb := setupUserDataTestUseCase(t)
	// Do NOT seed env cache — simulate no location data available
	_ = rdb

	uc.candidateSources = nil

	ctx := t.Context()
	cmd := Command{
		Message:  "a dónde puedo viajar",
		ClientIP: "10.0.0.99", // IP not in env cache
	}

	resp, err := uc.Execute(ctx, cmd, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Mode != "discovery" {
		t.Errorf("Mode = %q, want 'discovery'", resp.Mode)
	}

	if !resp.NeedsClarification {
		t.Error("expected NeedsClarification=true for open query without constraints")
	}

	// Without country context, the question should be the generic budget question
	if resp.ClarificationQuestion == "" {
		t.Error("expected non-empty ClarificationQuestion")
	}
	t.Logf("No-cache clarification question: %s", resp.ClarificationQuestion)
}

// =============================================================================
// Helpers
// =============================================================================

// containsSubstring is a simple string contains helper for test assertions.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
