// Tests para el formateador LLM del pipeline de discovery.
// Verifica la estructura del prompt generado con candidatos conocidos.
package ai_search

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// BuildFormattingPrompt tests
// =============================================================================

// sampleCandidates provee un conjunto conocido de candidatos para tests determinísticos.
func sampleCandidates() []Candidate {
	return []Candidate{
		{
			Destination: "Bali",
			Country:     "Indonesia",
			Region:      "southeast_asia",
			BudgetTier:  "low",
			BestMonths:  []int{4, 5, 6, 7, 8, 9, 10},
			Score:       0.85,
			Reasons:     []string{"destino popular entre viajeros", "temporada ideal en julio", "presupuesto accesible"},
		},
		{
			Destination: "Cancún",
			Country:     "México",
			Region:      "americas",
			BudgetTier:  "medium",
			BestMonths:  []int{11, 12, 1, 2, 3, 4},
			Score:       0.78,
			Reasons:     []string{"destino popular entre viajeros", "temporada aceptable en julio"},
		},
		{
			Destination: "Barcelona",
			Country:     "España",
			Region:      "europe",
			BudgetTier:  "high",
			BestMonths:  []int{5, 6, 9, 10},
			Score:       0.72,
			Reasons:     []string{"destino popular entre viajeros", "temporada ideal en julio", "coincide con tus destinos favoritos"},
		},
	}
}

func TestBuildFormattingPrompt_HasCandidates(t *testing.T) {
	rc := &RecommendationContext{
		Query:               "recomiéndame playa barato en verano",
		IntentConfidence:    0.85,
		RequiresClarification: false,
		RankedCandidates:    sampleCandidates(),
		ParsedConstraints: Constraints{
			Budget: "low",
			Season: "summer",
			Month:  7,
		},
	}

	prompt := BuildFormattingPrompt(rc, "es", "Argentina", "EUR")
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}

	// Verificar elementos requeridos en el prompt
	checks := []string{
		"No inventes",        // instrucción de no inventar
		"Bali",               // primer candidato
		"Indonesia",          // país del primer candidato
		"bajo",               // budget tier en español
		"Cancún",             // segundo candidato
		"Barcelona",          // tercer candidato
		"España",             // país del tercer candidato
	}

	for _, check := range checks {
		if !strings.Contains(strings.ToLower(prompt), strings.ToLower(check)) {
			t.Errorf("prompt missing expected content: %q\nPrompt:\n%s", check, prompt)
		}
	}

	// Verificar que NO menciona fechas fuera de la lista de candidatos
	if strings.Contains(prompt, "Madrid") {
		t.Error("prompt should not mention Madrid — not in candidate list")
	}
	if strings.Contains(prompt, "Tokio") {
		t.Error("prompt should not mention Tokio — not in candidate list")
	}
}

func TestBuildFormattingPrompt_EmptyCandidates(t *testing.T) {
	rc := &RecommendationContext{
		Query:               "viaje a marte",
		IntentConfidence:    0.3,
		RequiresClarification: false,
		RankedCandidates:    nil,
	}

	prompt := BuildFormattingPrompt(rc, "es", "Argentina", "EUR")
	if prompt == "" {
		t.Fatal("expected non-empty prompt even with no candidates")
	}

	// Debería contener un mensaje de "sin resultados"
	lower := strings.ToLower(prompt)
	if !strings.Contains(lower, "no encont") && !strings.Contains(lower, "sin result") {
		t.Errorf("expected honest no-results message, got: %s", prompt)
	}
}

func TestBuildFormattingPrompt_ClarificationMode(t *testing.T) {
	rc := &RecommendationContext{
		Query:                  "quiero viajar",
		IntentConfidence:       0.3,
		RequiresClarification:  true,
		ClarificationQuestion:  "¿Qué presupuesto tenés en mente?",
	}

	prompt := BuildFormattingPrompt(rc, "es", "Argentina", "EUR")
	if prompt == "" {
		t.Fatal("expected non-empty prompt for clarification mode")
	}

	// En modo clarificación, no debería mencionar candidatos
	if strings.Contains(prompt, "Bali") {
		t.Error("clarification prompt should not mention specific candidates")
	}

	// Debería ser una pregunta
	if !strings.Contains(prompt, "¿") {
		t.Errorf("clarification prompt should contain a question, got: %s", prompt)
	}
}

func TestBuildFormattingPrompt_HasBudgetTier(t *testing.T) {
	rc := &RecommendationContext{
		Query:          "destinos de lujo",
		IntentConfidence: 0.8,
		RankedCandidates: []Candidate{
			{
				Destination: "París",
				Country:     "Francia",
				BudgetTier:  "high",
				BestMonths:  []int{4, 5, 6, 9, 10},
				Score:       0.9,
				Reasons:     []string{"destino popular entre viajeros", "temporada ideal en mayo"},
			},
		},
	}

	prompt := BuildFormattingPrompt(rc, "es", "Argentina", "EUR")
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}

	// Debe traducir "high" a "alto" o "lujo"
	lower := strings.ToLower(prompt)
	if !strings.Contains(lower, "alto") && !strings.Contains(lower, "lujo") {
		t.Errorf("expected budget tier translated to Spanish, got: %s", prompt)
	}
}

func TestBuildFormattingPrompt_MultipleCandidates(t *testing.T) {
	// Verifica que el prompt incluye todos los candidatos provistos
	candidates := []Candidate{
		{Destination: "DestinoA", Country: "PaísA", BudgetTier: "low", BestMonths: []int{1}, Score: 0.9, Reasons: []string{"razón A"}},
		{Destination: "DestinoB", Country: "PaísB", BudgetTier: "medium", BestMonths: []int{6}, Score: 0.8, Reasons: []string{"razón B"}},
		{Destination: "DestinoC", Country: "PaísC", BudgetTier: "high", BestMonths: []int{12}, Score: 0.7, Reasons: []string{"razón C"}},
		{Destination: "DestinoD", Country: "PaísD", BudgetTier: "low", BestMonths: []int{3}, Score: 0.6, Reasons: []string{"razón D"}},
		{Destination: "DestinoE", Country: "PaísE", BudgetTier: "medium", BestMonths: []int{9}, Score: 0.5, Reasons: []string{"razón E"}},
	}

	rc := &RecommendationContext{
		Query:            "recomiéndame destinos",
		IntentConfidence: 0.8,
		RankedCandidates: candidates,
	}

	prompt := BuildFormattingPrompt(rc, "es", "Argentina", "EUR")

	for _, c := range candidates {
		if !strings.Contains(prompt, c.Destination) {
			t.Errorf("prompt should contain all candidate destinations, missing: %s", c.Destination)
		}
	}
}

// =============================================================================
// ISSUE 1: buildFallbackMessage — sin instrucciones de sistema
// =============================================================================

func TestBuildFallbackMessage_NoSystemInstructions(t *testing.T) {
	// ISSUE 1: La respuesta al usuario NO debe contener instrucciones del sistema.
	// "Solo describí", "No inventes", etc. son para el LLM, no para el usuario final.
	rc := &RecommendationContext{
		Query: "recomiéndame playa barato en verano",
		RankedCandidates: []Candidate{
			{Destination: "Cancún", Country: "México", BudgetTier: "medium", Source: "curated_beach",
				Score: 0.85, Reasons: []string{"destino popular entre viajeros", "temporada ideal en mayo"}},
			{Destination: "Bali", Country: "Indonesia", BudgetTier: "low", Source: "curated_beach",
				Score: 0.82, Reasons: []string{"destino popular entre viajeros", "presupuesto accesible"}},
			{Destination: "Tenerife", Country: "España", BudgetTier: "medium", Source: "curated_beach",
				Score: 0.79, Reasons: []string{"destino popular entre viajeros"}},
		},
	}

	msg := buildFallbackMessage(rc)

	if msg == "" {
		t.Fatal("expected non-empty fallback message")
	}

	// NO debe contener instrucciones del sistema del LLM
	forbidden := []string{
		"Solo describí",
		"No inventes",
		"Instrucciones",
		"Seleccioná los 3 a 5 mejores",
		"Mencioná: nombre del destino",
		"Sé útil y entusiasta",
		"system prompt",
	}
	for _, fb := range forbidden {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(fb)) {
			t.Errorf("fallback message should NOT contain system instruction %q. Got:\n%s", fb, msg)
		}
	}

	// DEBE contener nombres de destinos (es una recomendación real)
	foundCancun := strings.Contains(msg, "Cancún")
	foundBali := strings.Contains(msg, "Bali")
	if !foundCancun || !foundBali {
		t.Errorf("fallback message should mention destination names. Cancún=%v, Bali=%v. Got:\n%s",
			foundCancun, foundBali, msg)
	}

	// Debe ser español natural y legible
	if len(msg) < 20 {
		t.Errorf("fallback message too short (%d chars): %s", len(msg), msg)
	}
}

func TestBuildFallbackMessage_EmptyCandidates(t *testing.T) {
	rc := &RecommendationContext{
		Query:            "quiero irme",
		RankedCandidates: nil,
	}

	msg := buildFallbackMessage(rc)

	if msg == "" {
		t.Fatal("expected non-empty fallback message for empty candidates")
	}

	// Debe ser un mensaje honesto de "sin resultados"
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "no encont") && !strings.Contains(lower, "sin result") {
		t.Errorf("expected honest no-results message, got: %s", msg)
	}
}

func TestBuildFallbackMessage_ClarificationMode(t *testing.T) {
	rc := &RecommendationContext{
		Query:                  "quiero viajar",
		RequiresClarification:  true,
		ClarificationQuestion:  "¿Qué presupuesto tenés en mente?",
	}

	msg := buildFallbackMessage(rc)

	if msg == "" {
		t.Fatal("expected non-empty fallback message for clarification mode")
	}

	// En modo clarificación, la respuesta debe ser una pregunta
	if !strings.Contains(msg, "¿") {
		t.Errorf("clarification message should contain a question, got: %s", msg)
	}

	// No debe mencionar destinos
	if strings.Contains(msg, "Cancún") || strings.Contains(msg, "Bali") {
		t.Error("clarification message should NOT mention specific destinations")
	}
}

func TestBuildFallbackMessage_WordsPerCandidate(t *testing.T) {
	// Verifica que la respuesta menciona la mayoría de candidatos
	candidates := make([]Candidate, 5)
	for i := range 5 {
		candidates[i] = Candidate{
			Destination: fmt.Sprintf("Dest-%d", i),
			Country:     fmt.Sprintf("País-%d", i),
			BudgetTier:  "medium",
			Source:      "curated_beach",
			Score:       0.9 - float64(i)*0.05,
			Reasons:     []string{fmt.Sprintf("razón %d", i)},
		}
	}

	rc := &RecommendationContext{
		Query:            "recomiéndame destinos",
		RankedCandidates: candidates,
	}

	msg := buildFallbackMessage(rc)

	// Al menos 3 de los 5 candidatos deben ser mencionados
	count := 0
	for i := range 5 {
		if strings.Contains(msg, fmt.Sprintf("Dest-%d", i)) {
			count++
		}
	}
	if count < 3 {
		t.Errorf("fallback message only mentions %d/5 candidates, expected >= 3. Got:\n%s", count, msg)
	}
}
