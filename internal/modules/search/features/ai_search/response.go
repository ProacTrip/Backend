// DTO de respuesta para AI search — respuesta unificada.
// Combina estado de la conversación, intención interpretada,
// y resultados de búsqueda (vuelos y/o hoteles).
// También soporta el modo discovery con candidatos rankeados.
package ai_search

import (
	"encoding/json"
	"time"
)

// Response is the unified AI search API response.
// Contains conversation state, interpreted intent, AI response text,
// and search results (flights and/or hotels as RawMessage).
//
// FlightsError and HotelsError are populated when a "both" intent
// has one searcher fail while the other succeeds (partial results).
//
// Discovery-specific fields (Mode, Candidates, TotalCandidates, etc.)
// are populated when the discovery pipeline is active. They use omitzero
// so existing exact-search responses are unchanged.
type Response struct {
	ConversationID string          `json:"conversation_id"`
	TurnCount      int             `json:"turn_count"`
	MaxTurns       int             `json:"max_turns"`
	Intent         string          `json:"intent"`
	Confidence     float64         `json:"confidence"`
	Message        string          `json:"message"` // AI response text / follow-up question
	MissingFields  []string        `json:"missing_fields,omitzero"`
	Flights        json.RawMessage `json:"flights,omitzero"`
	Hotels         json.RawMessage `json:"hotels,omitzero"`
	FlightsError   string          `json:"flights_error,omitzero"` // non-empty when flights searcher failed in "both"
	HotelsError    string          `json:"hotels_error,omitzero"`  // non-empty when hotels searcher failed in "both"
	// FromCache indicates whether the AI INTERPRETATION was served from the blake3 cache.
	// It does NOT indicate whether search results (flights/hotels) came from cache.
	// For incomplete/ambiguous intents, this is always false — those are never cached.
	FromCache bool `json:"from_cache"`

	// --- Discovery fields (omitidos en exact search) ---

	// Mode indica el modo de búsqueda: "discovery" o "exact".
	Mode string `json:"mode,omitzero"`
	// Candidates son los destinos rankeados (top 3-5) del pipeline de discovery.
	Candidates []Candidate `json:"candidates,omitzero"`
	// TotalCandidates es el total de candidatos antes del truncado.
	TotalCandidates int `json:"total_candidates,omitzero"`
	// NeedsClarification indica si se necesita más información del usuario.
	NeedsClarification bool `json:"needs_clarification,omitzero"`
	// ClarificationQuestion es la pregunta de aclaración generada.
	ClarificationQuestion string `json:"clarification_question,omitzero"`
	// CachedAt es la marca de tiempo cuando la respuesta fue cacheada.
	CachedAt *time.Time `json:"cached_at,omitzero"`
}
