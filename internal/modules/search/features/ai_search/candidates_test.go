// Tests para las CandidateSource implementations del pipeline de discovery.
// Verifica la interfaz CandidateSource y sus 2 implementaciones:
// FavoritesSource, SavedSearchSource.
package ai_search

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// CandidateSource interface compliance — AR-005
// =============================================================================

// mockCandidateSource implementa CandidateSource para verificar la interfaz.
type mockCandidateSource struct {
	name   string
	cands  []Candidate
	err    error
}

func (m *mockCandidateSource) Name() string { return m.name }
func (m *mockCandidateSource) Generate(ctx context.Context, rc *RecommendationContext) ([]Candidate, error) {
	return m.cands, m.err
}

func TestCandidateSource_InterfaceCompliance(t *testing.T) {
	// Verifica que mockCandidateSource implementa CandidateSource
	var cs CandidateSource = &mockCandidateSource{name: "test"}
	if cs.Name() != "test" {
		t.Error("Name() mismatch")
	}
}

func TestCandidateSource_MockReturnsError(t *testing.T) {
	ctx := t.Context()
	wantErr := errors.New("source error")
	cs := &mockCandidateSource{name: "failing", err: wantErr}

	rc := &RecommendationContext{Query: "test"}
	cands, err := cs.Generate(ctx, rc)

	if !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
	if cands != nil {
		t.Error("expected nil candidates on error")
	}
}

func TestCandidateSource_MockReturnsCandidates(t *testing.T) {
	ctx := t.Context()
	wantCands := []Candidate{
		{Destination: "Bali", Country: "Indonesia", BudgetTier: "low"},
		{Destination: "Tokio", Country: "Japón", BudgetTier: "high"},
	}
	cs := &mockCandidateSource{name: "populated", cands: wantCands}

	rc := &RecommendationContext{Query: "playa"}
	cands, err := cs.Generate(ctx, rc)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(cands))
	}
	if cands[0].Destination != "Bali" {
		t.Errorf("cands[0].Destination = %q, want Bali", cands[0].Destination)
	}
}

// =============================================================================
// FavoritesSource — AR-006
// =============================================================================

func TestFavoritesSource_Name(t *testing.T) {
	src := &FavoritesSource{}
	if src.Name() != "favorites" {
		t.Errorf("Name() = %q, want 'favorites'", src.Name())
	}
}

func TestFavoritesSource_EmptyFavorites(t *testing.T) {
	ctx := t.Context()
	rc := &RecommendationContext{
		UserID: "user-1",
		Query:  "playa",
	}

	// Sin favoritos → slice vacío, sin error
	src := &FavoritesSource{getFavorites: func(userID string) []domain.SavedSearchData {
		return nil
	}}

	cands, err := src.Generate(ctx, rc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates with no favorites, got %d", len(cands))
	}
}

func TestFavoritesSource_WithFavorites(t *testing.T) {
	ctx := t.Context()
	rc := &RecommendationContext{
		UserID: "user-1",
		Query:  "viaje",
	}

	favData := []domain.SavedSearchData{
		{
			Name:       "Bali trip",
			Parameters: json.RawMessage(`{"destination":"Bali","country":"Indonesia"}`),
		},
		{
			Name:       "Tokyo adventure",
			Parameters: json.RawMessage(`{"destination":"Tokio","country":"Japón"}`),
		},
	}

	src := &FavoritesSource{getFavorites: func(userID string) []domain.SavedSearchData {
		if userID != "user-1" {
			t.Errorf("expected userID 'user-1', got %q", userID)
		}
		return favData
	}}

	cands, err := src.Generate(ctx, rc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(cands))
	}
	if cands[0].Destination != "Bali" {
		t.Errorf("cands[0].Destination = %q, want Bali", cands[0].Destination)
	}
	if cands[1].Country != "Japón" {
		t.Errorf("cands[1].Country = %q, want Japón", cands[1].Country)
	}
}

// =============================================================================
// SavedSearchSource — AR-007
// =============================================================================

func TestSavedSearchSource_Name(t *testing.T) {
	src := &SavedSearchSource{}
	if src.Name() != "saved_searches" {
		t.Errorf("Name() = %q, want 'saved_searches'", src.Name())
	}
}

func TestSavedSearchSource_EmptySearches(t *testing.T) {
	ctx := t.Context()
	rc := &RecommendationContext{
		UserID: "user-1",
	}

	src := &SavedSearchSource{getSavedSearches: func(userID string) []domain.SavedSearchData {
		return nil
	}}

	cands, err := src.Generate(ctx, rc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates with no saved searches, got %d", len(cands))
	}
}

func TestSavedSearchSource_ExtractsDestinations(t *testing.T) {
	ctx := t.Context()
	rc := &RecommendationContext{
		UserID: "user-1",
	}

	searches := []domain.SavedSearchData{
		{
			Name:       "Verano en México",
			Parameters: json.RawMessage(`{"destination":"Cancún","country":"México"}`),
		},
		{
			Name:       "Escape Europeo",
			Parameters: json.RawMessage(`{"destination":"París","country":"Francia"}`),
		},
	}

	src := &SavedSearchSource{getSavedSearches: func(userID string) []domain.SavedSearchData {
		return searches
	}}

	cands, err := src.Generate(ctx, rc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(cands))
	}
	if cands[0].Destination != "Cancún" {
		t.Errorf("cands[0].Destination = %q, want Cancún", cands[0].Destination)
	}
	if cands[1].Country != "Francia" {
		t.Errorf("cands[1].Country = %q, want Francia", cands[1].Country)
	}
}

func TestSavedSearchSource_AnonUser(t *testing.T) {
	ctx := t.Context()
	rc := &RecommendationContext{
		UserID: "", // usuario anónimo
	}

	src := &SavedSearchSource{getSavedSearches: func(userID string) []domain.SavedSearchData {
		return nil // userID vacío → sin búsquedas
	}}

	cands, err := src.Generate(ctx, rc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates for anonymous user, got %d", len(cands))
	}
}



// =============================================================================
// Helpers
// =============================================================================
