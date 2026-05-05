// DTO de respuesta para AI search — respuesta unificada.
// Combina estado de la conversación, intención interpretada,
// y resultados de búsqueda (vuelos y/o hoteles).
package ai_search

import (
	"encoding/json"
)

// Response is the unified AI search API response.
// Contains conversation state, interpreted intent, AI response text,
// and search results (flights and/or hotels as RawMessage).
//
// FlightsError and HotelsError are populated when a "both" intent
// has one searcher fail while the other succeeds (partial results).
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
}
