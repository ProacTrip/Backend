// Tipos del pipeline de discovery.
// Define las estructuras de datos para el modo discovery del AI search:
// candidatos, restricciones, resultado de intención, y contexto de recomendación.
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
	// Valores: "curated_beach", "curated_cheap", "curated_trending",
	// "user_favorite", "user_saved", "popularity", "seasonal".
	Source string `json:"source"`
}

// =============================================================================
// Constraints — restricciones extraídas de la consulta del usuario
// =============================================================================

// Constraints representa las restricciones de viaje extraídas del lenguaje natural.
type Constraints struct {
	Budget      string   `json:"budget,omitzero"`       // "low", "medium", "high"
	Season      string   `json:"season,omitzero"`       // "summer", "winter", etc
	TravelStyle []string `json:"travel_style,omitzero"` // "beach", "city", "nature"
	Region      string   `json:"region,omitzero"`
	Month       int      `json:"month,omitzero"` // 1-12, solo con mes explícito ("enero", "julio")
	// ClimateIntent convierte palabras de temporada en intención climática
	// multi-hemisferio. No dispara exclusión dura (solo afecta ranking).
	// Valores: "pleasant_warm", "cool_mild", "mild", "".
	ClimateIntent string `json:"climate_intent,omitzero"`
}

// =============================================================================
// IntentResult — resultado de la clasificación de intención
// =============================================================================

// IntentResult representa el resultado de la clasificación de intención del usuario.
type IntentResult struct {
	Mode                SearchMode `json:"mode"`
	Confidence           float64   `json:"confidence"`
	NeedsClarification   bool      `json:"needs_clarification"`
}

// =============================================================================
// RecommendationContext — contexto completo de una recomendación
// =============================================================================

// RecommendationContext agrupa toda la información necesaria para el pipeline de discovery.
type RecommendationContext struct {
	Query               string
	SearchMode          SearchMode
	IntentConfidence    float64
	UserID              string
	ClientIP            string
	GL                  string
	HL                  string
	Currency            string
	ParsedConstraints      Constraints
	Candidates             []Candidate
	RankedCandidates       []Candidate
	RequiresClarification  bool
	ClarificationQuestion  string
	// ClarificationRounds cuenta cuántas rondas de clarificación se han hecho.
	// Máximo 1 ronda — si ya se alcanzó, no se vuelve a preguntar.
	ClarificationRounds int
	// DetectedCountry es el nombre del país detectado desde el caché de entorno (env:{ip}).
	// Se usa para generar preguntas de clarificación contextuales.
	DetectedCountry string
	// DetectedCountryCode es el código ISO del país detectado desde el caché de entorno.
	DetectedCountryCode string
}
