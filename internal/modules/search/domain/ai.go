// Dominio de AI Search — tipos para interpretación de lenguaje natural,
// conversaciones multi-turno, y criterios de filtrado determinísticos.
//
// DESIGN: AIInterpreter es un puerto puro de parsing. No sabe nada de
// FlightProvider/HotelProvider, cache, rate limits, ni PG.
// El UseCase orquesta todo: interpreta → ejecuta búsquedas → filtra.
//
// FilterCriteria se aplica de forma determinística en Go POST-resultados.
// La AI NUNCA toca resultados — solo sugiere filtros desde el intent.
package domain

import (
	"context"
	"encoding/json"
	"time"
)

// =============================================================================
// AIInterpreter — Port for AI-based natural language parsing
// =============================================================================

// AIInterpreter is the port for AI natural language interpretation.
// It takes a user message, optional conversation history, and a language code
// (e.g., "es", "en") and returns a structured TravelIntent.
// Implementations adapt to specific AI backends (DeepSeek, Ollama, etc.).
type AIInterpreter interface {
	Parse(ctx context.Context, message string, history []ConversationMessage, language string) (*TravelIntent, error)
}

// =============================================================================
// TravelIntent — structured result of AI interpretation
// =============================================================================

// TravelIntent is the structured interpretation of a user's travel query.
// Produced by AIInterpreter.Parse(), validated and acted upon by the UseCase.
type TravelIntent struct {
	Type          string              `json:"type"`           // "flights"|"hotels"|"both"|"ambiguous"|"incomplete"
	Confidence    float64             `json:"confidence"`     // 0.0 to 1.0
	MissingFields []string            `json:"missing_fields"` // from AI, validated by UseCase
	FollowUp      string              `json:"follow_up"`      // AI-generated question for user
	FlightParams  *FlightSearchRequest `json:"flight_params,omitzero"`
	HotelParams   *HotelSearchRequest  `json:"hotel_params,omitzero"`
}

// =============================================================================
// ConversationState — active multi-turn conversation
// =============================================================================

// ConversationState tracks a multi-turn AI search conversation.
// Auth users get PG-backed persistence; anonymous users are Dragonfly-only.
type ConversationState struct {
	ID        string                `json:"id"`
	UserID    string                `json:"user_id,omitzero"` // empty for anon
	Messages  []ConversationMessage `json:"messages"`
	Intent    *TravelIntent         `json:"intent,omitzero"`
	Results   json.RawMessage       `json:"results,omitzero"`
	TurnCount int                   `json:"turn_count"`
	MaxTurns  int                   `json:"max_turns"`
	CreatedAt time.Time             `json:"created_at"`
	ExpiresAt time.Time             `json:"expires_at"`
}

// =============================================================================
// ConversationMessage — a single turn in the conversation
// =============================================================================

// ConversationMessage represents one message in a multi-turn AI conversation.
type ConversationMessage struct {
	Role      string    `json:"role"`      // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// =============================================================================
// FilterCriteria — deterministic Go filters applied post-search
// =============================================================================

// FilterCriteria holds filters suggested by the AI from user intent.
// These are applied deterministically in Go AFTER search results come back.
// The AI never touches results — it only suggests what to filter.
type FilterCriteria struct {
	// Hard filters (Go deterministic)
	MaxPrice  *float64 `json:"max_price,omitzero"`
	MinRating *float64 `json:"min_rating,omitzero"`
	Stops     *string  `json:"stops,omitzero"`
	Amenities []string `json:"amenities,omitzero"`

	// Soft filters (AI ranking suggestion)
	SortBy   string   `json:"sort_by,omitzero"`
	Keywords []string `json:"keywords,omitzero"`
}
