// Tests para los tipos del pipeline de discovery.
// Verifica JSON roundtrip y comportamiento omitzero de cada tipo.
package ai_search

import (
	"encoding/json"
	"testing"
)

// =============================================================================
// SearchMode — constantes y JSON
// =============================================================================

func TestSearchMode_Constants(t *testing.T) {
	// Verifica que las constantes sean distintas
	if SearchModeExact == SearchModeAssisted {
		t.Error("SearchModeExact y SearchModeAssisted no deben ser iguales")
	}
	if SearchModeExact == SearchModeDiscovery {
		t.Error("SearchModeExact y SearchModeDiscovery no deben ser iguales")
	}
	if SearchModeAssisted == SearchModeDiscovery {
		t.Error("SearchModeAssisted y SearchModeDiscovery no deben ser iguales")
	}
}

func TestSearchMode_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		mode SearchMode
		want string
	}{
		{"exact", SearchModeExact, `"exact"`},
		{"assisted", SearchModeAssisted, `"assisted"`},
		{"discovery", SearchModeDiscovery, `"discovery"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.mode)
			if err != nil {
				t.Fatalf("Marshal(%s) error: %v", tc.name, err)
			}
			if string(data) != tc.want {
				t.Errorf("Marshal(%s) = %s, want %s", tc.name, string(data), tc.want)
			}

			var decoded SearchMode
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", tc.name, err)
			}
			if decoded != tc.mode {
				t.Errorf("Unmarshal(Marshal(%s)) = %s, want %s", tc.name, decoded, tc.mode)
			}
		})
	}
}

// =============================================================================
// Candidate — JSON roundtrip y omitzero
// =============================================================================

func TestCandidate_JSONRoundtrip(t *testing.T) {
	c := Candidate{
		Destination: "Bali",
		Country:     "Indonesia",
		Region:      "southeast_asia",
		Tags:        []string{"beach", "wellness"},
		BudgetTier:  "low",
		BestMonths:  []int{4, 5, 6, 7, 8, 9, 10},
		Score:       0.85,
		Reasons:     []string{"Mejor temporada", "Presupuesto accesible"},
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Candidate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Destination != c.Destination {
		t.Errorf("Destination = %q, want %q", decoded.Destination, c.Destination)
	}
	if decoded.Score != c.Score {
		t.Errorf("Score = %f, want %f", decoded.Score, c.Score)
	}
	if len(decoded.Reasons) != len(c.Reasons) {
		t.Errorf("Reasons len = %d, want %d", len(decoded.Reasons), len(c.Reasons))
	}
	if len(decoded.Tags) != len(c.Tags) {
		t.Errorf("Tags len = %d, want %d", len(decoded.Tags), len(c.Tags))
	}
}

func TestCandidate_OmitZero(t *testing.T) {
	// Score=0 debe omitirse, Reasons vacío debe omitirse
	c := Candidate{
		Destination: "Tokio",
		Country:     "Japón",
		Region:      "east_asia",
		Tags:        []string{"city"},
		BudgetTier:  "high",
		BestMonths:  []int{3, 4, 10, 11},
		Score:       0,
		Reasons:     nil,
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	// Score=0 debe omitirse con omitzero
	if _, exists := raw["score"]; exists {
		t.Error("score con valor 0 no debería aparecer (omitzero)")
	}
	// reasons nil también debe omitirse
	if _, exists := raw["reasons"]; exists {
		t.Error("reasons nil no debería aparecer (omitzero)")
	}
	// Campos no-cero deben estar
	if _, exists := raw["destination"]; !exists {
		t.Error("destination debería estar presente")
	}
}

func TestCandidate_OmitZero_Present(t *testing.T) {
	// Score != 0 debe aparecer, Reasons con elementos debe aparecer
	c := Candidate{
		Destination: "París",
		Country:     "Francia",
		Region:      "europe",
		Tags:        []string{"culture", "romance"},
		BudgetTier:  "high",
		BestMonths:  []int{4, 5, 6, 9, 10},
		Score:       0.95,
		Reasons:     []string{"Imperdible en primavera"},
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	if _, exists := raw["score"]; !exists {
		t.Error("score != 0 debería aparecer")
	}
	if _, exists := raw["reasons"]; !exists {
		t.Error("reasons con elementos debería aparecer")
	}
}

// =============================================================================
// Constraints — JSON roundtrip y omitzero
// =============================================================================

func TestConstraints_JSONRoundtrip(t *testing.T) {
	con := Constraints{
		Budget:      "low",
		Season:      "summer",
		TravelStyle: []string{"beach", "nature"},
		Region:      "southeast_asia",
		Month:       7,
	}

	data, err := json.Marshal(con)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Constraints
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Budget != con.Budget {
		t.Errorf("Budget = %q, want %q", decoded.Budget, con.Budget)
	}
	if decoded.Month != con.Month {
		t.Errorf("Month = %d, want %d", decoded.Month, con.Month)
	}
	if len(decoded.TravelStyle) != len(con.TravelStyle) {
		t.Errorf("TravelStyle len = %d, want %d", len(decoded.TravelStyle), len(con.TravelStyle))
	}
}

func TestConstraints_OmitZero(t *testing.T) {
	// Campos vacíos/cero deben omitirse
	con := Constraints{
		Budget: "low",
	}

	data, err := json.Marshal(con)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	// budget="low" debe estar presente
	if _, exists := raw["budget"]; !exists {
		t.Error("budget debería estar presente")
	}
	// season="" debe omitirse
	if _, exists := raw["season"]; exists {
		t.Error("season vacío debería omitirse")
	}
	// travel_style nil debe omitirse
	if _, exists := raw["travel_style"]; exists {
		t.Error("travel_style nil debería omitirse")
	}
	// month=0 debe omitirse
	if _, exists := raw["month"]; exists {
		t.Error("month=0 debería omitirse")
	}
	// region="" debe omitirse
	if _, exists := raw["region"]; exists {
		t.Error("region vacío debería omitirse")
	}
}

// =============================================================================
// IntentResult — JSON roundtrip
// =============================================================================

func TestIntentResult_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name   string
		result IntentResult
	}{
		{
			name: "discovery con confianza alta",
			result: IntentResult{
				Mode:              SearchModeDiscovery,
				Confidence:        0.85,
				NeedsClarification: false,
			},
		},
		{
			name: "discovery con confianza baja",
			result: IntentResult{
				Mode:              SearchModeDiscovery,
				Confidence:        0.4,
				NeedsClarification: true,
			},
		},
		{
			name: "exact",
			result: IntentResult{
				Mode:              SearchModeExact,
				Confidence:        0.95,
				NeedsClarification: false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.result)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var decoded IntentResult
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if decoded.Mode != tc.result.Mode {
				t.Errorf("Mode = %s, want %s", decoded.Mode, tc.result.Mode)
			}
			if decoded.Confidence != tc.result.Confidence {
				t.Errorf("Confidence = %f, want %f", decoded.Confidence, tc.result.Confidence)
			}
			if decoded.NeedsClarification != tc.result.NeedsClarification {
				t.Errorf("NeedsClarification = %v, want %v", decoded.NeedsClarification, tc.result.NeedsClarification)
			}
		})
	}
}

// =============================================================================
// RecommendationContext — estructura compuesta
// =============================================================================

func TestCandidate_SourceField_JSONRoundtrip(t *testing.T) {
	// ISSUE 3: Verifica que el campo source está presente en el JSON.
	// Debe usar json:"source" sin omitzero porque siempre está poblado.
	tests := []struct {
		name   string
		c      Candidate
		expect string
	}{
		{
			name:   "curated_beach",
			c:      Candidate{Destination: "Cancún", Country: "México", Source: "curated_beach"},
			expect: "curated_beach",
		},
		{
			name:   "curated_cheap",
			c:      Candidate{Destination: "Tailandia", Country: "Tailandia", Source: "curated_cheap"},
			expect: "curated_cheap",
		},
		{
			name:   "curated_trending",
			c:      Candidate{Destination: "Tokio", Country: "Japón", Source: "curated_trending"},
			expect: "curated_trending",
		},
		{
			name:   "popularity",
			c:      Candidate{Destination: "París", Country: "Francia", Source: "popularity"},
			expect: "popularity",
		},
		{
			name:   "seasonal",
			c:      Candidate{Destination: "Bali", Country: "Indonesia", Source: "seasonal"},
			expect: "seasonal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.c)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("Unmarshal to map error: %v", err)
			}

			// Source siempre debe estar presente (sin omitzero)
			src, exists := raw["source"]
			if !exists {
				t.Fatal("source field missing from JSON — should be present with json:\"source\" (no omitzero)")
			}
			if src != tc.expect {
				t.Errorf("source = %q, want %q", src, tc.expect)
			}

			// Roundtrip: unmarshal back to struct
			var decoded Candidate
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal to struct error: %v", err)
			}
			if decoded.Source != tc.expect {
				t.Errorf("roundtrip Source = %q, want %q", decoded.Source, tc.expect)
			}
		})
	}
}

func TestCandidate_SourceField_Empty(t *testing.T) {
	// Cuando Source está vacío, el campo source debe aparecer como string vacío
	// (sin omitzero, no se omite nunca).
	c := Candidate{Destination: "Bali", Source: ""}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	// Con omitzero no se incluiría. Sin omitzero debe estar presente como "".
	if _, exists := raw["source"]; !exists {
		t.Error("source field should be present even when empty (no omitzero)")
	}
}

func TestRecommendationContext_Fields(t *testing.T) {
	rc := RecommendationContext{
		Query:              "playa barata en julio",
		SearchMode:         SearchModeDiscovery,
		IntentConfidence:   0.85,
		UserID:             "usr_123",
		ClientIP:           "192.168.1.1",
		GL:                 "AR",
		HL:                 "es",
		Currency:           "ARS",
		ParsedConstraints: Constraints{
			Budget: "low",
			Month:  7,
		},
		Candidates: []Candidate{
			{Destination: "Bali", Country: "Indonesia", BudgetTier: "low"},
		},
		RequiresClarification: false,
	}

	if rc.Query != "playa barata en julio" {
		t.Errorf("Query = %q, want %q", rc.Query, "playa barata en julio")
	}
	if rc.SearchMode != SearchModeDiscovery {
		t.Errorf("SearchMode = %s, want %s", rc.SearchMode, SearchModeDiscovery)
	}
	if len(rc.Candidates) != 1 {
		t.Errorf("Candidates len = %d, want 1", len(rc.Candidates))
	}
}
