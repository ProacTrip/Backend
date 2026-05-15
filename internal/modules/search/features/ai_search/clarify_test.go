// Tests para la estrategia de clarificación del pipeline de discovery.
// Verifica NeedsClarification y GenerateClarificationQuestion con 10+ escenarios.
package ai_search

import (
	"testing"
)

// =============================================================================
// NeedsClarification tests
// =============================================================================

func TestNeedsClarification_AllFalse(t *testing.T) {
	// Caso: confianza alta, pocos candidatos, constraints presentes, sin flag NeedsClarification
	rc := &RecommendationContext{
		IntentConfidence:     0.8,
		RequiresClarification: false,
		ParsedConstraints: Constraints{
			Budget: "low",
			Season: "summer",
		},
		Candidates: []Candidate{
			{Destination: "A", Score: 0.90},
			{Destination: "B", Score: 0.70},
			{Destination: "C", Score: 0.50},
			{Destination: "D", Score: 0.40},
			{Destination: "E", Score: 0.30},
		},
	}

	if NeedsClarification(rc) {
		t.Error("expected no clarification needed when confidence is high and constraints exist")
	}
}

func TestNeedsClarification_LowConfidenceManyCandidates(t *testing.T) {
	// IntentConfidence < 0.5 AND candidates > 10 → necesita clarificación
	rc := &RecommendationContext{
		IntentConfidence:     0.3,
		RequiresClarification: false,
		ParsedConstraints: Constraints{
			Budget: "low",
		},
		Candidates: make([]Candidate, 15),
	}

	if !NeedsClarification(rc) {
		t.Error("expected clarification needed when confidence < 0.5 and candidates > 10")
	}
}

func TestNeedsClarification_LowConfidenceButFewCandidates(t *testing.T) {
	// IntentConfidence < 0.5 pero pocos candidatos → NO necesita clarificación
	rc := &RecommendationContext{
		IntentConfidence:     0.3,
		RequiresClarification: false,
		ParsedConstraints: Constraints{
			Budget: "low",
		},
		Candidates: []Candidate{
			{Destination: "A", Score: 0.90},
			{Destination: "B", Score: 0.70},
			{Destination: "C", Score: 0.50},
			{Destination: "D", Score: 0.40},
			{Destination: "E", Score: 0.30},
		},
	}

	if NeedsClarification(rc) {
		t.Error("expected no clarification when confidence is low but candidates are few (≤10)")
	}
}

func TestNeedsClarification_FromIntentDetection(t *testing.T) {
	// NeedsClarification=true desde la detección de intención → necesita clarificación
	rc := &RecommendationContext{
		IntentConfidence:     0.8,
		RequiresClarification: true,
		ParsedConstraints: Constraints{
			Budget: "low",
			Season: "summer",
		},
		Candidates: make([]Candidate, 5),
	}

	if !NeedsClarification(rc) {
		t.Error("expected clarification needed when NeedsClarification flag from intent is true")
	}
}

func TestNeedsClarification_EmptyConstraints(t *testing.T) {
	// Constraints vacíos (sin budget, season, style, region) → necesita clarificación
	rc := &RecommendationContext{
		IntentConfidence:     0.6,
		RequiresClarification: false,
		ParsedConstraints:     Constraints{},
		Candidates:            make([]Candidate, 5),
	}

	if !NeedsClarification(rc) {
		t.Error("expected clarification needed when all constraints are empty")
	}
}

func TestNeedsClarification_OnlyBudgetConstraint(t *testing.T) {
	// Al menos una constraint presente → NO necesita clarificación por constraints vacíos
	rc := &RecommendationContext{
		IntentConfidence:     0.6,
		RequiresClarification: false,
		ParsedConstraints: Constraints{
			Budget: "low",
		},
		Candidates: []Candidate{
			{Destination: "A", Score: 0.90},
			{Destination: "B", Score: 0.70},
			{Destination: "C", Score: 0.50},
			{Destination: "D", Score: 0.40},
			{Destination: "E", Score: 0.30},
		},
	}

	if NeedsClarification(rc) {
		t.Error("expected no clarification when at least one constraint is present")
	}
}

func TestNeedsClarification_MaxRoundsReached(t *testing.T) {
	// ClarificationRounds >= 1 → no pedir más clarificación (best-effort)
	rc := &RecommendationContext{
		IntentConfidence:     0.3,
		RequiresClarification: true,
		ParsedConstraints:     Constraints{},
		Candidates:            make([]Candidate, 15),
		ClarificationRounds:   1,
	}

	if NeedsClarification(rc) {
		t.Error("expected no clarification when max rounds reached despite all flags triggering")
	}
}

func TestNeedsClarification_MaxRoundsExceeded(t *testing.T) {
	// ClarificationRounds > 1 → no pedir más clarificación
	rc := &RecommendationContext{
		IntentConfidence:     0.3,
		RequiresClarification: true,
		ParsedConstraints:     Constraints{},
		Candidates:            make([]Candidate, 15),
		ClarificationRounds:   2,
	}

	if NeedsClarification(rc) {
		t.Error("expected no clarification when rounds exceeded max (2 > 1)")
	}
}

func TestNeedsClarification_ZeroCandidates(t *testing.T) {
	// Sin candidatos generados → necesita clarificación pero no por las reglas normales
	// Debe pasar a través y ser manejado por el pipeline (no-results)
	rc := &RecommendationContext{
		IntentConfidence:     0.3,
		RequiresClarification: false,
		ParsedConstraints:     Constraints{},
		Candidates:            nil,
	}

	// Constraints vacíos → sigue necesitando clarificación
	if !NeedsClarification(rc) {
		t.Error("expected clarification needed when constraints are empty with 0 candidates")
	}
}

// =============================================================================
// CRITICAL 4: Score spread rule — top-3 score spread < 0.15 → clarification
// =============================================================================

func TestCheckScoreSpread_Low(t *testing.T) {
	// Top 3 scores: 0.80, 0.75, 0.70 — spread=0.10 < 0.15 → necesita clarificación
	ranked := []Candidate{
		{Destination: "A", Score: 0.80},
		{Destination: "B", Score: 0.75},
		{Destination: "C", Score: 0.70},
		{Destination: "D", Score: 0.50},
	}

	if !checkScoreSpread(ranked) {
		t.Error("expected clarification needed when top-3 score spread < 0.15 (ambiguous recommendations)")
	}
}

func TestCheckScoreSpread_High(t *testing.T) {
	// Top 3 scores: 0.90, 0.70, 0.50 — spread=0.40 ≥ 0.15 → NO necesita clarificación
	ranked := []Candidate{
		{Destination: "A", Score: 0.90},
		{Destination: "B", Score: 0.70},
		{Destination: "C", Score: 0.50},
	}

	if checkScoreSpread(ranked) {
		t.Error("expected NO clarification when top-3 score spread ≥ 0.15 (clear winner)")
	}
}

func TestCheckScoreSpread_NotEnoughCandidates(t *testing.T) {
	// Solo 2 candidatos — no se puede calcular spread de top-3 → NO activa
	ranked := []Candidate{
		{Destination: "A", Score: 0.80},
		{Destination: "B", Score: 0.79},
	}

	if checkScoreSpread(ranked) {
		t.Error("expected no clarification when <3 candidates (score spread rule not applicable)")
	}
}

func TestCheckScoreSpread_ExactThreshold(t *testing.T) {
	// Top 3 scores: 0.85, 0.72, 0.70 — spread=0.15 (exacto) → NO necesita clarificación
	ranked := []Candidate{
		{Destination: "A", Score: 0.85},
		{Destination: "B", Score: 0.72},
		{Destination: "C", Score: 0.70},
	}

	if checkScoreSpread(ranked) {
		t.Error("expected NO clarification when score spread == 0.15 (threshold is exclusive: < 0.15)")
	}
}

// =============================================================================
// GenerateClarificationQuestion tests
// =============================================================================

func TestGenerateClarificationQuestion_MissingBudget(t *testing.T) {
	rc := &RecommendationContext{
		Query:              "quiero viajar",
		IntentConfidence:   0.4,
		RequiresClarification: true,
		ParsedConstraints:  Constraints{},
	}

	q := GenerateClarificationQuestion(rc)
	if q == "" {
		t.Fatal("expected non-empty clarification question")
	}
	// Debería mencionar presupuesto
	if !contains(q, "presupuesto") && !contains(q, "gastar") && !contains(q, "económico") {
		t.Errorf("expected budget-related question, got: %q", q)
	}
}

func TestGenerateClarificationQuestion_MissingStyle(t *testing.T) {
	rc := &RecommendationContext{
		Query:              "viaje a algún lado",
		IntentConfidence:   0.4,
		RequiresClarification: true,
		ParsedConstraints: Constraints{
			Budget: "low",
		},
	}

	q := GenerateClarificationQuestion(rc)
	if q == "" {
		t.Fatal("expected non-empty clarification question")
	}
	// Debería mencionar estilo (playa, ciudad, montaña)
	if !contains(q, "playa") && !contains(q, "ciudad") && !contains(q, "montaña") && !contains(q, "estilo") {
		t.Errorf("expected style-related question, got: %q", q)
	}
}

func TestGenerateClarificationQuestion_MissingAll(t *testing.T) {
	rc := &RecommendationContext{
		Query:              "no sé a dónde ir",
		IntentConfidence:   0.3,
		RequiresClarification: true,
		ParsedConstraints:  Constraints{},
	}

	q := GenerateClarificationQuestion(rc)
	if q == "" {
		t.Fatal("expected non-empty clarification question when all constraints missing")
	}
	// Debería ser una pregunta general
	if len(q) < 10 {
		t.Errorf("question too short for completely empty constraints: %q", q)
	}
}

func TestGenerateClarificationQuestion_HasAllConstraints(t *testing.T) {
	rc := &RecommendationContext{
		Query:              "playa barata en europa en verano",
		IntentConfidence:   0.4,
		RequiresClarification: true,
		ParsedConstraints: Constraints{
			Budget:      "low",
			Season:      "summer",
			TravelStyle: []string{"beach"},
			Region:      "europe",
		},
	}

	q := GenerateClarificationQuestion(rc)
	if q == "" {
		t.Fatal("expected non-empty question even with all constraints")
	}
	// Con todas las constraints, la pregunta debería ser genérica
	if !contains(q, "¿") {
		t.Errorf("expected a question format, got: %q", q)
	}
}

// =============================================================================
// Helper
// =============================================================================

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
