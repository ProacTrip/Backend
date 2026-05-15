// Tests para el detector de intención de búsqueda.
// Verifica ClassifyIntent — clasificación de consultas en Discovery, Assisted y Exact.
// Cubre 18 casos: discovery explícito, asistido, búsqueda exacta, edge cases.
package ai_search

import (
	"testing"
)

func TestClassifyIntent_Discovery(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantMode    SearchMode
		minConf     float64
		maxConf     float64 // 0 means no max constraint
		wantClarify bool
	}{
		{
			name:        "recomienda keyword",
			query:       "recomiéndame un destino para viajar",
			wantMode:    SearchModeDiscovery,
			minConf:     0.6,
			wantClarify: false,
		},
		{
			name:        "sugerime keyword",
			query:       "sugerime a dónde puedo ir de vacaciones",
			wantMode:    SearchModeDiscovery,
			minConf:     0.6,
			wantClarify: true,
		},
		{
			name:        "ideas para keyword",
			query:       "ideas para viajar en verano",
			wantMode:    SearchModeDiscovery,
			minConf:     0.6,
			wantClarify: false,
		},
		{
			name:        "a dónde keyword",
			query:       "a dónde puedo viajar en julio que sea barato",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			wantClarify: true,
		},
		{
			name:        "dónde viajar keyword",
			query:       "dónde viajar en invierno",
			wantMode:    SearchModeDiscovery,
			minConf:     0.6,
			wantClarify: true,
		},
		{
			name:        "escapar keyword",
			query:       "necesito escapar de la rutina",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			wantClarify: true,
		},
		{
			name:        "vacaciones keyword",
			query:       "vacaciones en playa para dos personas",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			wantClarify: false,
		},
		{
			name:        "playa en keyword",
			query:       "playa en el caribe en diciembre",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			wantClarify: false,
		},
		{
			name:        "barato en keyword",
			query:       "algo barato en europa para el verano",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			wantClarify: false,
		},
		{
			name:        "algún lado keyword",
			query:       "quiero ir a algún lado que tenga playa",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			wantClarify: true,
		},
		{
			name:        "cualquier parte keyword",
			query:       "viaje a cualquier parte en agosto",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			wantClarify: true,
		},
		{
			name:        "multiple discovery keywords — alta confianza",
			query:       "recomiéndame vacaciones en playa baratas, a dónde puedo ir",
			wantMode:    SearchModeDiscovery,
			minConf:     0.8,
			wantClarify: true,
		},
		// CRITICAL 1: "viaje" alone → Discovery, low confidence, needs clarification
		{
			name:        "viaje alone — debe ser Discovery con baja confianza",
			query:       "viaje",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			maxConf:     0.7,
			wantClarify: true, // Single-word queries are too ambiguous
		},
		{
			name:        "viajar keyword",
			query:       "quiero viajar a algún lado",
			wantMode:    SearchModeDiscovery,
			minConf:     0.5,
			wantClarify: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyIntent(tt.query)

			if result.Mode != tt.wantMode {
				t.Errorf("Mode = %q, want %q", result.Mode, tt.wantMode)
			}
			if result.Confidence < tt.minConf {
				t.Errorf("Confidence = %.2f, want >= %.2f", result.Confidence, tt.minConf)
			}
			if tt.maxConf > 0 && result.Confidence > tt.maxConf {
				t.Errorf("Confidence = %.2f, want <= %.2f", result.Confidence, tt.maxConf)
			}
			if result.Confidence > 1.0 {
				t.Errorf("Confidence = %.2f exceeds max 1.0", result.Confidence)
			}
			if result.NeedsClarification != tt.wantClarify {
				t.Errorf("NeedsClarification = %v, want %v", result.NeedsClarification, tt.wantClarify)
			}
		})
	}
}

func TestClassifyIntent_Assisted(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantMode SearchMode
		minConf float64
	}{
		{
			name:    "parecido a keyword",
			query:  "algo parecido a Bali pero más barato",
			wantMode: SearchModeAssisted,
			minConf: 0.5,
		},
		{
			name:    "similar a keyword",
			query:  "destino similar a Cancún",
			wantMode: SearchModeAssisted,
			minConf: 0.5,
		},
		{
			name:    "tipo keyword",
			query:  "tipo Tailandia pero en América",
			wantMode: SearchModeAssisted,
			minConf: 0.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyIntent(tt.query)

			if result.Mode != tt.wantMode {
				t.Errorf("Mode = %q, want %q", result.Mode, tt.wantMode)
			}
			if result.Confidence < tt.minConf {
				t.Errorf("Confidence = %.2f, want >= %.2f", result.Confidence, tt.minConf)
			}
		})
	}
}

func TestClassifyIntent_Exact(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		minConf float64
	}{
		{
			name:    "vuelo concreto",
			query:   "vuelo a Madrid el 15 de junio",
			minConf: 1.0, // CRITICAL 2: no discovery keywords → C=1.0
		},
		{
			name:    "hotel concreto",
			query:   "hotel en Barcelona para dos personas",
			minConf: 1.0,
		},
		{
			name:    "consulta corta sin keywords",
			query:   "Madrid",
			minConf: 1.0,
		},
		{
			name:    "empty query",
			query:   "",
			minConf: 0, // special case: empty → C=0
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyIntent(tt.query)

			if result.Mode != SearchModeExact {
				t.Errorf("Mode = %q, want %q for query %q", result.Mode, SearchModeExact, tt.query)
			}
			// Exact queries should have high confidence when no discovery keywords present
			if result.Confidence < tt.minConf {
				t.Errorf("Confidence = %.2f, want >= %.2f", result.Confidence, tt.minConf)
			}
		})
	}
}

func TestClassifyIntent_NeedsClarification(t *testing.T) {
	// Verifica que las consultas abiertas marcan NeedsClarification
	openQueries := []string{
		"a dónde puedo ir",
		"algún lado para viajar",
		"cualquier parte con playa",
	}

	for _, q := range openQueries {
		t.Run("open_"+q[:min(15, len(q))], func(t *testing.T) {
			result := ClassifyIntent(q)
			if !result.NeedsClarification {
				t.Errorf("query %q should need clarification, got %v", q, result.NeedsClarification)
			}
		})
	}
}

func TestClassifyIntent_ConfidenceBounds(t *testing.T) {
	// Verifica que la confianza está siempre en [0, 1]
	queries := []string{
		"recomiéndame un destino barato en playa para vacaciones de verano escapando del frío",
		"vuelo a Madrid el 15 de junio",
		"",
		"algo parecido a Bali",
		"a dónde ir en enero",
	}

	for _, q := range queries {
		t.Run("bounds_"+q[:min(20, len(q))], func(t *testing.T) {
			result := ClassifyIntent(q)
			if result.Confidence < 0 || result.Confidence > 1.0 {
				t.Errorf("Confidence out of bounds: %.2f for query %q", result.Confidence, q)
			}
		})
	}
}

func TestClassifyIntent_DiscoveryOverAssisted(t *testing.T) {
	// Cuando hay keywords tanto de discovery como de assisted,
	// discovery debe ganar (es más fuerte).
	result := ClassifyIntent("recomiéndame algo parecido a Bali")
	if result.Mode != SearchModeDiscovery {
		t.Errorf("Discovery keywords should take precedence over assisted, got %q", result.Mode)
	}
}
