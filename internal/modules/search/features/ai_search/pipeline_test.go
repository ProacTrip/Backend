// Tests de integración para el pipeline simplificado de discovery.
// Verifica RunDiscoveryPipeline: intent, constraints, candidates (user data),
// clarificación — sin hard filters, ranking, diversity, ni curated fallback.
package ai_search

import (
	"testing"
)

// =============================================================================
// Helpers
// =============================================================================

// setupTestUseCase crea un UseCase sin CandidateSources para tests del pipeline.
func setupTestUseCase(t *testing.T) *UseCase {
	t.Helper()

	uc := NewUseCase(UseCaseDeps{
		DiscoveryEnabled: true,
	})
	// Sin CandidateSources — el pipeline opera con los candidatos seteados en RC
	return uc
}

// setupTestRC crea un RecommendationContext para tests del pipeline.
func setupTestRC(query string) *RecommendationContext {
	return &RecommendationContext{
		Query:    query,
		UserID:   "test-user",
		ClientIP: "127.0.0.1",
	}
}

// =============================================================================
// RunDiscoveryPipeline tests — simplified pipeline
// =============================================================================

func TestRunDiscoveryPipeline_BaratoEnVerano(t *testing.T) {
	uc := setupTestUseCase(t)
	ctx := t.Context()
	rc := setupTestRC("barato en verano")

	err := uc.RunDiscoveryPipeline(ctx, rc)
	if err != nil {
		t.Fatalf("RunDiscoveryPipeline error: %v", err)
	}

	// Verificar intent detection
	if rc.SearchMode != SearchModeDiscovery {
		t.Errorf("expected Discovery mode, got %q", rc.SearchMode)
	}

	// Verificar constraints
	if rc.ParsedConstraints.Budget != "low" {
		t.Errorf("expected budget=low, got %q", rc.ParsedConstraints.Budget)
	}
	if rc.ParsedConstraints.Season != "summer" {
		t.Errorf("expected season=summer, got %q", rc.ParsedConstraints.Season)
	}
}

func TestRunDiscoveryPipeline_RecomiendaPlayaEuropa(t *testing.T) {
	uc := setupTestUseCase(t)
	ctx := t.Context()
	rc := setupTestRC("recomiéndame playa en europa")

	err := uc.RunDiscoveryPipeline(ctx, rc)
	if err != nil {
		t.Fatalf("RunDiscoveryPipeline error: %v", err)
	}

	if rc.SearchMode != SearchModeDiscovery {
		t.Errorf("expected Discovery mode, got %q", rc.SearchMode)
	}

	// Verificar región y estilo
	if rc.ParsedConstraints.Region != "europe" {
		t.Errorf("expected region=europe, got %q", rc.ParsedConstraints.Region)
	}

	foundBeach := false
	for _, s := range rc.ParsedConstraints.TravelStyle {
		if s == "beach" {
			foundBeach = true
			break
		}
	}
	if !foundBeach {
		t.Errorf("expected travel_style to include beach, got %v", rc.ParsedConstraints.TravelStyle)
	}
}

func TestRunDiscoveryPipeline_QuieroViajarAlgunLado(t *testing.T) {
	uc := setupTestUseCase(t)
	ctx := t.Context()
	rc := setupTestRC("quiero viajar a algún lado")

	err := uc.RunDiscoveryPipeline(ctx, rc)
	if err != nil {
		t.Fatalf("RunDiscoveryPipeline error: %v", err)
	}

	if rc.SearchMode != SearchModeDiscovery {
		t.Errorf("expected Discovery mode, got %q", rc.SearchMode)
	}

	// Consulta muy abierta → necesita aclaración (desde ClassifyIntent)
	if !rc.RequiresClarification {
		t.Error("expected NeedsClarification=true for open-ended query")
	}
}

func TestRunDiscoveryPipeline_ExactSearchMode(t *testing.T) {
	uc := setupTestUseCase(t)
	ctx := t.Context()
	rc := setupTestRC("vuelo a Madrid el 15 de junio")

	err := uc.RunDiscoveryPipeline(ctx, rc)
	if err != nil {
		t.Fatalf("RunDiscoveryPipeline error: %v", err)
	}

	// Debería clasificarse como ExactSearch
	if rc.SearchMode != SearchModeExact {
		t.Errorf("expected ExactSearch mode for flight query, got %q", rc.SearchMode)
	}

	// Para ExactSearch, el pipeline no produce RankedCandidates
}

func TestRunDiscoveryPipeline_EmptyQuery(t *testing.T) {
	uc := setupTestUseCase(t)
	ctx := t.Context()
	rc := setupTestRC("")

	err := uc.RunDiscoveryPipeline(ctx, rc)
	if err != nil {
		t.Fatalf("RunDiscoveryPipeline with empty query should not error: %v", err)
	}
	// Consulta vacía → ExactSearch mode con 0 confianza
	if rc.SearchMode != SearchModeExact {
		t.Errorf("expected ExactSearch for empty query, got %q", rc.SearchMode)
	}
}

// =============================================================================
// Clarification tests
// =============================================================================

func TestPipeline_QuieroIrme_ClarificationTriggered(t *testing.T) {
	uc := setupTestUseCase(t)
	uc.clarifyEnabled = true
	ctx := t.Context()
	rc := &RecommendationContext{
		Query:    "recomiéndame un lugar",
		UserID:   "test-user",
		ClientIP: "127.0.0.1",
	}

	err := uc.RunDiscoveryPipeline(ctx, rc)
	if err != nil {
		t.Fatalf("RunDiscoveryPipeline error: %v", err)
	}

	if rc.SearchMode != SearchModeDiscovery {
		t.Errorf("expected Discovery mode, got %q", rc.SearchMode)
	}

	// "recomiéndame un lugar" es ultra ambiguo → NeedsClarification desde intent
	if !rc.RequiresClarification {
		t.Error("expected RequiresClarification=true for ultra-ambiguous query")
	}

	if rc.ClarificationQuestion == "" {
		t.Error("expected clarification question")
	}

	// No debe devolver candidatos cuando necesita clarificación
	if len(rc.RankedCandidates) > 0 {
		t.Errorf("expected no ranked candidates during clarification, got %d", len(rc.RankedCandidates))
	}
}

func TestPipeline_ClarificationDisabled(t *testing.T) {
	// Con clarificación deshabilitada, no se pide aclaración aunque sería necesaria
	uc := setupTestUseCase(t)
	uc.clarifyEnabled = false
	ctx := t.Context()
	rc := &RecommendationContext{
		Query:    "recomiéndame un lugar",
		UserID:   "test-user",
	}

	err := uc.RunDiscoveryPipeline(ctx, rc)
	if err != nil {
		t.Fatalf("RunDiscoveryPipeline error: %v", err)
	}

	if rc.SearchMode != SearchModeDiscovery {
		t.Errorf("expected Discovery mode, got %q", rc.SearchMode)
	}

	// Clarificación deshabilitada → no se debería haber activado
	if rc.ClarificationRounds > 0 {
		t.Errorf("expected no clarification rounds when clarifyEnabled=false, got %d", rc.ClarificationRounds)
	}
}
