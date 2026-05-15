// Tests para el extractor de restricciones de consulta en lenguaje natural.
// Verifica ExtractConstraints — presupuesto, temporada, estilo, región y mes.
package ai_search

import (
	"testing"
)

func TestExtractConstraints_Budget(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantBudget string
	}{
		{
			name:       "barato keyword",
			query:      "destino barato en playa",
			wantBudget: "low",
		},
		{
			name:       "económico keyword",
			query:      "viaje económico a europa",
			wantBudget: "low",
		},
		{
			name:       "lujo keyword",
			query:      "hotel de lujo en París",
			wantBudget: "high",
		},
		{
			name:       "sin presupuesto",
			query:      "vuelo a Madrid",
			wantBudget: "",
		},
		{
			name:       "medio keyword",
			query:      "presupuesto medio para vacaciones",
			wantBudget: "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ExtractConstraints(tt.query)
			if c.Budget != tt.wantBudget {
				t.Errorf("Budget = %q, want %q", c.Budget, tt.wantBudget)
			}
		})
	}
}

func TestExtractConstraints_Season(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantSeason string
	}{
		{
			name:       "verano keyword",
			query:      "vacaciones de verano",
			wantSeason: "summer",
		},
		{
			name:       "invierno keyword",
			query:      "escapada de invierno",
			wantSeason: "winter",
		},
		{
			name:       "primavera keyword",
			query:      "viaje en primavera",
			wantSeason: "spring",
		},
		{
			name:       "otoño keyword",
			query:      "otoño en europa",
			wantSeason: "fall",
		},
		{
			name:       "sin temporada",
			query:      "vuelo a Madrid",
			wantSeason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ExtractConstraints(tt.query)
			if c.Season != tt.wantSeason {
				t.Errorf("Season = %q, want %q", c.Season, tt.wantSeason)
			}
		})
	}
}

func TestExtractConstraints_TravelStyle(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStyles []string
	}{
		{
			name:       "playa keyword",
			query:      "quiero playa en el caribe",
			wantStyles: []string{"beach"},
		},
		{
			name:       "montaña keyword",
			query:      "viaje a la montaña",
			wantStyles: []string{"nature"},
		},
		{
			name:       "ciudad keyword",
			query:      "ciudad europea para visitar",
			wantStyles: []string{"city"},
		},
		{
			name:       "cultural keyword",
			query:      "viaje cultural por asia",
			wantStyles: []string{"culture"},
		},
		{
			name:       "multiple styles",
			query:      "playa y ciudad en verano",
			wantStyles: []string{"beach", "city"},
		},
		{
			name:       "sin estilo",
			query:      "vuelo a Madrid",
			wantStyles: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ExtractConstraints(tt.query)
			if len(c.TravelStyle) != len(tt.wantStyles) {
				t.Fatalf("TravelStyle len = %d, want %d: %v", len(c.TravelStyle), len(tt.wantStyles), c.TravelStyle)
			}
			for i, s := range c.TravelStyle {
				if s != tt.wantStyles[i] {
					t.Errorf("TravelStyle[%d] = %q, want %q", i, s, tt.wantStyles[i])
				}
			}
		})
	}
}

func TestExtractConstraints_Region(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantRegion string
	}{
		{
			name:       "europa keyword",
			query:      "viaje barato a europa",
			wantRegion: "europe",
		},
		{
			name:       "asia keyword",
			query:      "destinos en asia",
			wantRegion: "asia",
		},
		{
			name:       "américa keyword",
			query:      "recorrer américa en verano",
			wantRegion: "americas",
		},
		{
			name:       "américa latina",
			query:      "américa latina mochilera",
			wantRegion: "americas",
		},
		{
			name:       "caribe keyword",
			query:      "playas del caribe",
			wantRegion: "caribbean",
		},
		{
			name:       "sin región",
			query:      "vuelo a Madrid",
			wantRegion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ExtractConstraints(tt.query)
			if c.Region != tt.wantRegion {
				t.Errorf("Region = %q, want %q", c.Region, tt.wantRegion)
			}
		})
	}
}

func TestExtractConstraints_Month(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantMonth int
	}{
		{
			name:      "enero",
			query:     "viaje en enero a la playa",
			wantMonth: 1,
		},
		{
			name:      "febrero",
			query:     "escapada en febrero",
			wantMonth: 2,
		},
		{
			name:      "marzo",
			query:     "vacaciones de marzo",
			wantMonth: 3,
		},
		{
			name:      "julio",
			query:     "barato en julio",
			wantMonth: 7,
		},
		{
			name:      "diciembre",
			query:     "viaje en diciembre",
			wantMonth: 12,
		},
		{
			name:      "sin mes",
			query:     "vuelo a Madrid",
			wantMonth: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ExtractConstraints(tt.query)
			if c.Month != tt.wantMonth {
				t.Errorf("Month = %d, want %d", c.Month, tt.wantMonth)
			}
		})
	}
}

func TestExtractConstraints_Combined(t *testing.T) {
	// Verifica que se extraen múltiples restricciones simultáneamente
	c := ExtractConstraints("barato en julio playa en europa")

	if c.Budget != "low" {
		t.Errorf("Budget = %q, want low", c.Budget)
	}
	if c.Month != 7 {
		t.Errorf("Month = %d, want 7", c.Month)
	}
	if c.Region != "europe" {
		t.Errorf("Region = %q, want europe", c.Region)
	}
	if c.Season != "" {
		t.Errorf("Season = %q, want empty (month is explicit)", c.Season)
	}
}

func TestExtractConstraints_EmptyQuery(t *testing.T) {
	// Consulta vacía no debe causar pánico
	c := ExtractConstraints("")
	if c.Budget != "" || c.Season != "" || c.Month != 0 || c.Region != "" {
		t.Error("empty query should return zero Constraints")
	}
}

// =============================================================================
// P15-004: ClimateIntent extraction
// =============================================================================

func TestExtractConstraints_ClimateIntent(t *testing.T) {
	tests := []struct {
		name              string
		query             string
		wantClimateIntent string
		wantSeason        string
		wantMonth         int
	}{
		{
			name:              "verano → pleasant_warm",
			query:             "playa barato en verano",
			wantClimateIntent: "pleasant_warm",
			wantSeason:        "summer",
			wantMonth:         0,
		},
		{
			name:              "invierno → cool_mild",
			query:             "escapada de invierno",
			wantClimateIntent: "cool_mild",
			wantSeason:        "winter",
			wantMonth:         0,
		},
		{
			name:              "primavera → mild",
			query:             "viaje en primavera",
			wantClimateIntent: "mild",
			wantSeason:        "spring",
			wantMonth:         0,
		},
		{
			name:              "otoño → mild",
			query:             "otoño en europa",
			wantClimateIntent: "mild",
			wantSeason:        "fall",
			wantMonth:         0,
		},
		{
			name:              "mes explícito → sin ClimateIntent",
			query:             "viaje en julio",
			wantClimateIntent: "",
			wantSeason:        "",
			wantMonth:         7,
		},
		{
			name:              "sin temporada → sin ClimateIntent",
			query:             "vuelo a Madrid",
			wantClimateIntent: "",
			wantSeason:        "",
			wantMonth:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ExtractConstraints(tt.query)
			if c.ClimateIntent != tt.wantClimateIntent {
				t.Errorf("ClimateIntent = %q, want %q", c.ClimateIntent, tt.wantClimateIntent)
			}
			if c.Season != tt.wantSeason {
				t.Errorf("Season = %q, want %q", c.Season, tt.wantSeason)
			}
			if c.Month != tt.wantMonth {
				t.Errorf("Month = %d, want %d", c.Month, tt.wantMonth)
			}
		})
	}
}
