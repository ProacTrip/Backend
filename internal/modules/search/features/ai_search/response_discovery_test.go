// Tests para el formato de respuesta de discovery.
// Verifica JSON roundtrip y comportamiento omitzero de los campos de discovery.
package ai_search

import (
	"encoding/json"
	"testing"
	"time"
)

// =============================================================================
// Response con campos de discovery — JSON roundtrip
// =============================================================================

func TestResponse_DiscoveryFields_Roundtrip(t *testing.T) {
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)

	resp := Response{
		ConversationID: "conv_123",
		TurnCount:      1,
		MaxTurns:       5,
		Intent:         "discovery",
		Confidence:     0.85,
		Message:        "Te recomiendo visitar Bali, Cancún y Barcelona...",
		Mode:           "discovery",
		Candidates: []Candidate{
			{Destination: "Bali", Country: "Indonesia", Score: 0.85, Reasons: []string{"temporada ideal"}},
			{Destination: "Cancún", Country: "México", Score: 0.78, Reasons: []string{"popular"}},
		},
		TotalCandidates:      10,
		NeedsClarification:   false,
		ClarificationQuestion: "",
		FromCache:            false,
		CachedAt:             &now,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Mode != resp.Mode {
		t.Errorf("Mode = %q, want %q", decoded.Mode, resp.Mode)
	}
	if len(decoded.Candidates) != len(resp.Candidates) {
		t.Errorf("Candidates len = %d, want %d", len(decoded.Candidates), len(resp.Candidates))
	}
	if decoded.Candidates[0].Destination != resp.Candidates[0].Destination {
		t.Errorf("first candidate Destination = %q, want %q",
			decoded.Candidates[0].Destination, resp.Candidates[0].Destination)
	}
	if decoded.TotalCandidates != resp.TotalCandidates {
		t.Errorf("TotalCandidates = %d, want %d", decoded.TotalCandidates, resp.TotalCandidates)
	}
	if decoded.NeedsClarification != resp.NeedsClarification {
		t.Errorf("NeedsClarification = %v, want %v", decoded.NeedsClarification, resp.NeedsClarification)
	}
	if decoded.FromCache != resp.FromCache {
		t.Errorf("FromCache = %v, want %v", decoded.FromCache, resp.FromCache)
	}
	if decoded.CachedAt == nil {
		t.Error("CachedAt should not be nil")
	} else if !decoded.CachedAt.Equal(now) {
		t.Errorf("CachedAt = %v, want %v", decoded.CachedAt, now)
	}
}

func TestResponse_DiscoveryFields_OmitZero(t *testing.T) {
	// Campos con omitzero: mode="", candidates=nil, total_candidates=0,
	// needs_clarification=false, clarification_question="", cached_at=nil
	resp := Response{
		ConversationID:       "conv_456",
		TurnCount:            1,
		MaxTurns:             5,
		Intent:               "flights",
		Confidence:           0.95,
		Message:              "Encontré 10 vuelos.",
		Mode:                 "",
		Candidates:           nil,
		TotalCandidates:      0,
		NeedsClarification:   false,
		ClarificationQuestion: "",
		FromCache:            false,
		CachedAt:             nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	// Campos que NO deben aparecer con omitzero
	omittedFields := []string{"mode", "candidates", "total_candidates", "needs_clarification", "clarification_question", "cached_at"}
	for _, field := range omittedFields {
		if _, exists := raw[field]; exists {
			t.Errorf("field %q should be omitted (omitzero), but was present", field)
		}
	}

	// Campos que SÍ deben aparecer
	requiredFields := []string{"conversation_id", "turn_count", "intent", "message", "from_cache"}
	for _, field := range requiredFields {
		if _, exists := raw[field]; !exists {
			t.Errorf("field %q should be present", field)
		}
	}
}

func TestResponse_DiscoveryFields_Present(t *testing.T) {
	// Campos con valores no-zero deben aparecer
	resp := Response{
		ConversationID:       "conv_789",
		TurnCount:            1,
		MaxTurns:             5,
		Intent:               "discovery",
		Confidence:           0.85,
		Message:              "Acá van los destinos...",
		Mode:                 "discovery",
		Candidates:           []Candidate{{Destination: "Bali", Score: 0.85}},
		TotalCandidates:      5,
		NeedsClarification:   true,
		ClarificationQuestion: "¿Qué presupuesto tenés?",
		FromCache:            false,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	// Campos que deben aparecer
	presentFields := []string{"mode", "candidates", "total_candidates", "needs_clarification", "clarification_question"}
	for _, field := range presentFields {
		if _, exists := raw[field]; !exists {
			t.Errorf("field %q should be present when non-zero", field)
		}
	}

	if mode, ok := raw["mode"]; !ok || mode != "discovery" {
		t.Errorf("mode = %v, want 'discovery'", mode)
	}
}
