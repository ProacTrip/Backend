package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Compile-time interface satisfaction checks
// =============================================================================

// testAIInterpreter is a mock that satisfies the AIInterpreter interface.
type testAIInterpreter struct{}

func (a *testAIInterpreter) Parse(_ context.Context, _ string, _ []ConversationMessage, _ string) (*TravelIntent, error) {
	return nil, nil
}

var _ AIInterpreter = (*testAIInterpreter)(nil)

// =============================================================================
// AIInterpreter interface runtime check
// =============================================================================

func TestAIInterpreter_MockSatisfiesInterface(t *testing.T) {
	var p AIInterpreter = &testAIInterpreter{}
	ctx := t.Context()

	intent, err := p.Parse(ctx, "quiero un vuelo a Madrid", nil, "es")
	if err != nil {
		t.Errorf("Parse returned unexpected error: %v", err)
	}
	if intent != nil {
		t.Error("Parse expected nil intent from mock")
	}
}

// =============================================================================
// TravelIntent JSON roundtrip
// =============================================================================

func TestTravelIntent_JSONRoundtrip_FlightOnly(t *testing.T) {
	original := TravelIntent{
		Type:          "flights",
		Confidence:    0.95,
		MissingFields: []string{"return_date"},
		FollowUp:      "¿Cuándo pensás volver?",
		FlightParams: &FlightSearchRequest{
			TripType:     "round_trip",
			Departure:    "EZE",
			Arrival:      "MAD",
			OutboundDate: "2026-06-15",
			Adults:       1,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped TravelIntent
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.Type != original.Type {
		t.Errorf("Type = %q, want %q", roundtripped.Type, original.Type)
	}
	if roundtripped.Confidence != original.Confidence {
		t.Errorf("Confidence = %v, want %v", roundtripped.Confidence, original.Confidence)
	}
	if len(roundtripped.MissingFields) != len(original.MissingFields) {
		t.Errorf("MissingFields len = %d, want %d", len(roundtripped.MissingFields), len(original.MissingFields))
	}
	if roundtripped.FollowUp != original.FollowUp {
		t.Errorf("FollowUp = %q, want %q", roundtripped.FollowUp, original.FollowUp)
	}
	if roundtripped.FlightParams == nil {
		t.Fatal("FlightParams should not be nil after roundtrip")
	}
	if roundtripped.FlightParams.Departure != original.FlightParams.Departure {
		t.Errorf("FlightParams.Departure = %q, want %q",
			roundtripped.FlightParams.Departure, original.FlightParams.Departure)
	}
	if roundtripped.HotelParams != nil {
		t.Error("HotelParams should be nil for flight-only intent")
	}
}

func TestTravelIntent_JSONRoundtrip_HotelOnly(t *testing.T) {
	original := TravelIntent{
		Type:          "hotels",
		Confidence:    0.88,
		MissingFields: nil,
		FollowUp:      "",
		HotelParams: &HotelSearchRequest{
			Query:        "Barcelona",
			CheckInDate:  "2026-07-01",
			CheckOutDate: "2026-07-05",
			Adults:       2,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped TravelIntent
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.Type != original.Type {
		t.Errorf("Type = %q, want %q", roundtripped.Type, original.Type)
	}
	if roundtripped.HotelParams == nil {
		t.Fatal("HotelParams should not be nil after roundtrip")
	}
	if roundtripped.HotelParams.Query != original.HotelParams.Query {
		t.Errorf("HotelParams.Query = %q, want %q",
			roundtripped.HotelParams.Query, original.HotelParams.Query)
	}
	if roundtripped.FlightParams != nil {
		t.Error("FlightParams should be nil for hotel-only intent")
	}
}

func TestTravelIntent_JSONRoundtrip_Both(t *testing.T) {
	original := TravelIntent{
		Type:       "both",
		Confidence: 0.92,
		FlightParams: &FlightSearchRequest{
			TripType:     "round_trip",
			Departure:    "EZE",
			Arrival:      "BCN",
			OutboundDate: "2026-08-10",
			ReturnDate:   "2026-08-20",
			Adults:       2,
		},
		HotelParams: &HotelSearchRequest{
			Query:        "Barcelona",
			CheckInDate:  "2026-08-10",
			CheckOutDate: "2026-08-20",
			Adults:       2,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped TravelIntent
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.FlightParams == nil {
		t.Error("FlightParams should not be nil for 'both' intent")
	}
	if roundtripped.HotelParams == nil {
		t.Error("HotelParams should not be nil for 'both' intent")
	}
}

func TestTravelIntent_JSONRoundtrip_Ambiguous(t *testing.T) {
	original := TravelIntent{
		Type:          "ambiguous",
		Confidence:    0.45,
		MissingFields: []string{"destination", "dates"},
		FollowUp:      "¿Querés buscar vuelos, hoteles, o ambos? ¿A dónde vas?",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped TravelIntent
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.Type != "ambiguous" {
		t.Errorf("Type = %q, want 'ambiguous'", roundtripped.Type)
	}
	if roundtripped.FlightParams != nil {
		t.Error("FlightParams should be nil for ambiguous intent")
	}
	if roundtripped.HotelParams != nil {
		t.Error("HotelParams should be nil for ambiguous intent")
	}
}

func TestTravelIntent_JSONRoundtrip_Incomplete(t *testing.T) {
	original := TravelIntent{
		Type:          "incomplete",
		Confidence:    0.30,
		MissingFields: []string{"outbound_date", "destination", "adults"},
		FollowUp:      "¿Desde dónde salís, a dónde vas, y en qué fecha?",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("serialized JSON should not be empty")
	}

	var roundtripped TravelIntent
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.Type != "incomplete" {
		t.Errorf("Type = %q, want 'incomplete'", roundtripped.Type)
	}
	if roundtripped.FollowUp == "" {
		t.Error("FollowUp should not be empty for incomplete intent")
	}
	if len(roundtripped.MissingFields) != 3 {
		t.Errorf("MissingFields len = %d, want 3", len(roundtripped.MissingFields))
	}
}

// =============================================================================
// ConversationState marshaling
// =============================================================================

func TestConversationState_JSONRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(10 * time.Minute)

	original := ConversationState{
		ID:     "conv-abc123",
		UserID: "user-456",
		Messages: []ConversationMessage{
			{Role: "user", Content: "vuelo a Madrid", Timestamp: now},
			{Role: "assistant", Content: "¿Qué fecha?", Timestamp: now.Add(time.Second)},
		},
		Intent: &TravelIntent{
			Type:       "flights",
			Confidence: 0.85,
			FlightParams: &FlightSearchRequest{
				Departure:    "EZE",
				Arrival:      "MAD",
				OutboundDate: "2026-07-01",
				Adults:       1,
			},
		},
		TurnCount: 2,
		MaxTurns:  5,
		CreatedAt: now,
		ExpiresAt: expires,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped ConversationState
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.ID != original.ID {
		t.Errorf("ID = %q, want %q", roundtripped.ID, original.ID)
	}
	if roundtripped.UserID != original.UserID {
		t.Errorf("UserID = %q, want %q", roundtripped.UserID, original.UserID)
	}
	if roundtripped.TurnCount != original.TurnCount {
		t.Errorf("TurnCount = %d, want %d", roundtripped.TurnCount, original.TurnCount)
	}
	if roundtripped.MaxTurns != original.MaxTurns {
		t.Errorf("MaxTurns = %d, want %d", roundtripped.MaxTurns, original.MaxTurns)
	}
	if !roundtripped.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", roundtripped.CreatedAt, original.CreatedAt)
	}
	if !roundtripped.ExpiresAt.Equal(original.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", roundtripped.ExpiresAt, original.ExpiresAt)
	}
	if len(roundtripped.Messages) != 2 {
		t.Errorf("Messages len = %d, want 2", len(roundtripped.Messages))
	}
	if roundtripped.Intent == nil {
		t.Fatal("Intent should not be nil after roundtrip")
	}
	if roundtripped.Intent.Type != "flights" {
		t.Errorf("Intent.Type = %q, want 'flights'", roundtripped.Intent.Type)
	}
}

func TestConversationState_AnonymousUser(t *testing.T) {
	original := ConversationState{
		ID:        "conv-anon-001",
		UserID:    "", // anonymous
		TurnCount: 1,
		MaxTurns:  3,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Anonymous: user_id should be omitted from JSON
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	// user_id field should NOT be present in empty string case (omitzero)
	if _, exists := raw["user_id"]; exists {
		t.Error("user_id should be omitted for anonymous user (empty string)")
	}

	var roundtripped ConversationState
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.UserID != "" {
		t.Errorf("UserID = %q, want empty", roundtripped.UserID)
	}
}

func TestConversationMessage_JSONRoundtrip(t *testing.T) {
	ts := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		msg  ConversationMessage
	}{
		{
			name: "user message",
			msg:  ConversationMessage{Role: "user", Content: "vuelo a Madrid en junio", Timestamp: ts},
		},
		{
			name: "assistant message",
			msg:  ConversationMessage{Role: "assistant", Content: "¿Ida y vuelta o solo ida?", Timestamp: ts},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.msg)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			var roundtripped ConversationMessage
			if err := json.Unmarshal(data, &roundtripped); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if roundtripped.Role != tc.msg.Role {
				t.Errorf("Role = %q, want %q", roundtripped.Role, tc.msg.Role)
			}
			if roundtripped.Content != tc.msg.Content {
				t.Errorf("Content = %q, want %q", roundtripped.Content, tc.msg.Content)
			}
			if !roundtripped.Timestamp.Equal(tc.msg.Timestamp) {
				t.Errorf("Timestamp = %v, want %v", roundtripped.Timestamp, tc.msg.Timestamp)
			}
		})
	}
}

// =============================================================================
// FilterCriteria validation
// =============================================================================

func TestFilterCriteria_JSONRoundtrip(t *testing.T) {
	original := FilterCriteria{
		MaxPrice:  new(500.00),
		MinRating: new(4.0),
		Stops:     new("direct_only"),
		Amenities: []string{"WiFi", "Pool", "Gym"},
		SortBy:    "price_asc",
		Keywords:  []string{"céntrico", "vista al mar"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped FilterCriteria
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.MaxPrice == nil {
		t.Fatal("MaxPrice should not be nil")
	}
	if *roundtripped.MaxPrice != *original.MaxPrice {
		t.Errorf("MaxPrice = %v, want %v", *roundtripped.MaxPrice, *original.MaxPrice)
	}
	if *roundtripped.MinRating != *original.MinRating {
		t.Errorf("MinRating = %v, want %v", *roundtripped.MinRating, *original.MinRating)
	}
	if *roundtripped.Stops != *original.Stops {
		t.Errorf("Stops = %q, want %q", *roundtripped.Stops, *original.Stops)
	}
	if len(roundtripped.Amenities) != len(original.Amenities) {
		t.Errorf("Amenities len = %d, want %d", len(roundtripped.Amenities), len(original.Amenities))
	}
	if roundtripped.SortBy != original.SortBy {
		t.Errorf("SortBy = %q, want %q", roundtripped.SortBy, original.SortBy)
	}
}

func TestFilterCriteria_EmptyFieldsOmitted(t *testing.T) {
	original := FilterCriteria{
		SortBy: "relevance",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	// These fields should be omitted when zero/nil
	for _, field := range []string{"max_price", "min_rating", "stops", "amenities", "keywords"} {
		if _, exists := raw[field]; exists {
			t.Errorf("%q should be omitted when empty", field)
		}
	}

	// SortBy should be present (non-zero string)
	if v, ok := raw["sort_by"]; !ok {
		t.Error("sort_by should be present")
	} else if v != "relevance" {
		t.Errorf("sort_by = %v, want 'relevance'", v)
	}
}

func TestFilterCriteria_NilSafety(t *testing.T) {
	// Verify zero-value FilterCriteria marshals without panics
	var fc FilterCriteria
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("marshal of zero FilterCriteria failed: %v", err)
	}
	// Empty struct with all omitzero fields → should be "{}"
	if string(data) != "{}" {
		t.Errorf("empty FilterCriteria JSON = %q, want '{}'", string(data))
	}
}

// =============================================================================
// Error sentinels
// =============================================================================

func TestErrAIErrorSentinels(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrAIUnavailable", ErrAIUnavailable, "AI_UNAVAILABLE: el servicio de IA no está disponible"},
		{"ErrAIParseFailure", ErrAIParseFailure, "AI_PARSE_FAILURE: la IA devolvió una respuesta inválida o malformada"},
		{"ErrConversationNotFound", ErrConversationNotFound, "CONVERSATION_NOT_FOUND: conversation_id no encontrado"},
		{"ErrTurnLimitExceeded", ErrTurnLimitExceeded, "TURN_LIMIT_EXCEEDED: se alcanzó el límite máximo de turnos"},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Error() != tc.msg {
				t.Errorf("%s.Error() = %q, want %q", tc.name, tc.err.Error(), tc.msg)
			}
		})
	}
}

func TestErrAIErrorSentinelsAreDistinct(t *testing.T) {
	aiErrors := []error{
		ErrAIUnavailable,
		ErrAIParseFailure,
		ErrConversationNotFound,
		ErrTurnLimitExceeded,
	}

	for i, e1 := range aiErrors {
		for j, e2 := range aiErrors {
			if i != j && errors.Is(e1, e2) {
				t.Errorf("error %d and %d should be distinct: %v == %v", i, j, e1, e2)
			}
		}
	}
}

func TestErrAIErrorSentinels_WrappedDetection(t *testing.T) {
	tests := []struct {
		name     string
		wrapped  error
		sentinel error
		want     bool
	}{
		{
			name:     "ErrAIUnavailable wrapped",
			wrapped:  fmt.Errorf("deepseek adapter: %w", ErrAIUnavailable),
			sentinel: ErrAIUnavailable,
			want:     true,
		},
		{
			name:     "ErrConversationNotFound wrapped",
			wrapped:  fmt.Errorf("dragonfly store: %w", ErrConversationNotFound),
			sentinel: ErrConversationNotFound,
			want:     true,
		},
		{
			name:     "cross-sentinel mismatch",
			wrapped:  fmt.Errorf("some error: %w", ErrTurnLimitExceeded),
			sentinel: ErrAIUnavailable,
			want:     false,
		},
		{
			name:     "non-AI sentinel mismatch",
			wrapped:  fmt.Errorf("provider issue: %w", ErrProviderUnavailable),
			sentinel: ErrAIUnavailable,
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.wrapped, tc.sentinel); got != tc.want {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tc.wrapped, tc.sentinel, got, tc.want)
			}
		})
	}
}
