// Tipos del pipeline de discovery.
// Define las estructuras de datos para el modo discovery del AI search:
// candidatos y modos de búsqueda.
package ai_search

// =============================================================================
// SearchMode — modos de búsqueda del sistema
// =============================================================================

// SearchMode representa el modo de búsqueda detectado a partir de la consulta del usuario.
type SearchMode string

const (
	// SearchModeExact — búsqueda concreta con parámetros específicos (vuelos, hoteles).
	SearchModeExact SearchMode = "exact"
	// SearchModeAssisted — el usuario necesita ayuda para definir su búsqueda.
	SearchModeAssisted SearchMode = "assisted"
	// SearchModeDiscovery — el usuario quiere descubrir destinos sin criterios concretos.
	SearchModeDiscovery SearchMode = "discovery"
)

// =============================================================================
// Candidate — un destino candidato para recomendar
// =============================================================================

// Candidate representa un destino turístico candidato generado por una CandidateSource.
type Candidate struct {
	Destination  string   `json:"destination"`
	Country      string   `json:"country"`
	Region       string   `json:"region"`
	Tags         []string `json:"tags"`
	BudgetTier   string   `json:"budget_tier"`
	BestMonths   []int    `json:"best_months"`
	AvoidMonths  []int    `json:"avoid_months,omitzero"`
	SafetyScore  float64  `json:"safety_score,omitzero"`
	VisaRequired bool     `json:"visa_required,omitzero"`
	Score        float64  `json:"score,omitzero"`
	Reasons      []string `json:"reasons,omitzero"`
	// RelaxedSeason indica que el candidato fue recuperado mediante relaxed filtering
	// (porque strict filters lo habían excluido por temporada o presupuesto).
	RelaxedSeason bool `json:"relaxed_season,omitzero"`
	// Source identifica la fuente de datos que generó este candidato.
	Source string `json:"source"`
}
