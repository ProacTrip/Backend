// Tests for DeepSeek adapter — mock HTTP server, JSON repair, retry, interface check.
package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Compile-time interface check (runtime confirmation)
// =============================================================================

func TestAdapterSatisfiesAIInterpreter(t *testing.T) {
	// Compile-time check is in adapter.go:
	//   var _ domain.AIInterpreter = (*Adapter)(nil)
	var a *Adapter
	var p domain.AIInterpreter = a
	_ = p
}

// =============================================================================
// Test helpers
// =============================================================================

// newTestAdapter creates an Adapter backed by an httptest server with the given handler.
func newTestAdapter(handler http.HandlerFunc) (*Adapter, *httptest.Server) {
	srv := httptest.NewServer(handler)
	client := NewClient("test-key", 5*time.Second,
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	return NewAdapter(client), srv
}

// validFlightIntentJSON returns valid JSON for a flights intent.
const validFlightIntentJSON = `{
	"type": "flights",
	"confidence": 0.95,
	"missing_fields": [],
	"follow_up": "",
	"flight_params": {
		"departure": "EZE",
		"arrival": "MAD",
		"outbound_date": "2026-06-15",
		"return_date": "2026-07-01",
		"adults": 2,
		"trip_type": "round_trip"
	}
}`

// validHotelIntentJSON returns valid JSON for a hotels intent.
const validHotelIntentJSON = `{
	"type": "hotels",
	"confidence": 0.88,
	"flight_params": null,
	"hotel_params": {
		"query": "Barcelona",
		"check_in_date": "2026-07-01",
		"check_out_date": "2026-07-05",
		"adults": 2,
		"rating": 4
	}
}`

// validBothIntentJSON returns valid JSON for a both intent.
const validBothIntentJSON = `{
	"type": "both",
	"confidence": 0.92,
	"flight_params": {
		"departure": "EZE",
		"arrival": "BCN",
		"outbound_date": "2026-08-10",
		"return_date": "2026-08-20",
		"adults": 2
	},
	"hotel_params": {
		"query": "Barcelona",
		"check_in_date": "2026-08-10",
		"check_out_date": "2026-08-20",
		"adults": 2
	}
}`

// validIncompleteIntentJSON returns valid JSON for an incomplete intent.
const validIncompleteIntentJSON = `{
	"type": "incomplete",
	"confidence": 0.30,
	"missing_fields": ["outbound_date", "destination"],
	"follow_up": "¿Desde dónde salís y en qué fecha?",
	"flight_params": null,
	"hotel_params": null
}`

// openAIChatResponse wraps JSON content in the OpenAI-compatible response format.
func openAIChatResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id": "chatcmpl-123",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     150,
			"completion_tokens": 80,
			"total_tokens":      230,
		},
	}
}

// =============================================================================
// Task 4.5.1 — Valid JSON response → correct TravelIntent
// =============================================================================

func TestParse_ValidFlightIntent(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(validFlightIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo de Buenos Aires a Madrid ida y vuelta en junio", nil, "es")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if intent.Type != "flights" {
		t.Errorf("Type = %q, want 'flights'", intent.Type)
	}
	if intent.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", intent.Confidence)
	}
	if intent.FlightParams == nil {
		t.Fatal("FlightParams should not be nil")
	}
	if intent.FlightParams.Departure != "EZE" {
		t.Errorf("Departure = %q, want 'EZE'", intent.FlightParams.Departure)
	}
	if intent.FlightParams.Arrival != "MAD" {
		t.Errorf("Arrival = %q, want 'MAD'", intent.FlightParams.Arrival)
	}
	if intent.FlightParams.OutboundDate != "2026-06-15" {
		t.Errorf("OutboundDate = %q, want '2026-06-15'", intent.FlightParams.OutboundDate)
	}
	if intent.FlightParams.ReturnDate != "2026-07-01" {
		t.Errorf("ReturnDate = %q, want '2026-07-01'", intent.FlightParams.ReturnDate)
	}
	if intent.FlightParams.Adults != 2 {
		t.Errorf("Adults = %d, want 2", intent.FlightParams.Adults)
	}
	if intent.FlightParams.TripType != "round_trip" {
		t.Errorf("TripType = %q, want 'round_trip'", intent.FlightParams.TripType)
	}
}

func TestParse_ValidHotelIntent(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(validHotelIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "hoteles en Barcelona en julio", nil, "es")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if intent.Type != "hotels" {
		t.Errorf("Type = %q, want 'hotels'", intent.Type)
	}
	if intent.HotelParams == nil {
		t.Fatal("HotelParams should not be nil")
	}
	if intent.HotelParams.Query != "Barcelona" {
		t.Errorf("Query = %q, want 'Barcelona'", intent.HotelParams.Query)
	}
	if intent.HotelParams.Adults != 2 {
		t.Errorf("Adults = %d, want 2", intent.HotelParams.Adults)
	}
	if intent.FlightParams != nil {
		t.Error("FlightParams should be nil for hotel-only intent")
	}
}

func TestParse_ValidBothIntent(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(validBothIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "viaje a Barcelona en agosto con hotel", nil, "es")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if intent.Type != "both" {
		t.Errorf("Type = %q, want 'both'", intent.Type)
	}
	if intent.FlightParams == nil {
		t.Fatal("FlightParams should not be nil for 'both' intent")
	}
	if intent.HotelParams == nil {
		t.Fatal("HotelParams should not be nil for 'both' intent")
	}
}

func TestParse_ValidIncompleteIntent(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(validIncompleteIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "quiero viajar", nil, "es")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if intent.Type != "incomplete" {
		t.Errorf("Type = %q, want 'incomplete'", intent.Type)
	}
	if len(intent.MissingFields) != 2 {
		t.Errorf("MissingFields len = %d, want 2", len(intent.MissingFields))
	}
	if intent.FollowUp == "" {
		t.Error("FollowUp should not be empty for incomplete intent")
	}
}

// =============================================================================
// Task 4.5.2 — Malformed JSON → repair + retry
// =============================================================================

func TestParse_MalformedJSON_RepairsAndSucceeds(t *testing.T) {
	// First response: JSON wrapped in markdown code block
	// repairJSON extracts it → succeeds on first call, no retry needed
	callCount := 0
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(
			"```json\n" + validFlightIntentJSON + "\n```",
		))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo EZE-MAD", nil, "es")

	if err != nil {
		t.Fatalf("Parse should succeed after repair, got: %v", err)
	}
	if intent.Type != "flights" {
		t.Errorf("Type = %q, want 'flights'", intent.Type)
	}
	// repairJSON extracts from markdown → parsed in first attempt
	if callCount != 1 {
		t.Errorf("Expected 1 call (repair in-call, no retry needed), got %d", callCount)
	}
}

func TestParse_MalformedJSON_ExhaustsRetries(t *testing.T) {
	// Always return malformed JSON — exhaust maxRetries (2)
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse("not json at all!!!"))
	})
	defer srv.Close()

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail after exhausting retries")
	}
	if !strings.Contains(err.Error(), "AI_PARSE_FAILURE") {
		t.Errorf("Expected AI_PARSE_FAILURE in error, got: %v", err)
	}
}

func TestParse_TrailingCommaJSON_Repaired(t *testing.T) {
	// JSON with trailing comma: {"type": "flights", "confidence": 0.9,}
	trailingCommaJSON := `{
	"type": "flights",
	"confidence": 0.9,
	"flight_params": {
		"departure": "EZE",
		"arrival": "MAD",
		"outbound_date": "2026-06-15",
		"adults": 1,
	},
}`

	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(trailingCommaJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err != nil {
		t.Fatalf("Parse should repair trailing commas, got: %v", err)
	}
	if intent.Type != "flights" {
		t.Errorf("Type = %q, want 'flights'", intent.Type)
	}
}

func TestParse_TextAroundJSON_Extracted(t *testing.T) {
	// JSON with text before and after
	textAroundJSON := "Claro, aquí está el JSON:\n" + validFlightIntentJSON + "\nEspero que te sirva."

	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(textAroundJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err != nil {
		t.Fatalf("Parse should extract JSON from surrounding text, got: %v", err)
	}
	if intent.Type != "flights" {
		t.Errorf("Type = %q, want 'flights'", intent.Type)
	}
}

// =============================================================================
// Task 4.5.3 — Timeout → error
// =============================================================================

func TestParse_Timeout_ReturnsError(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		// Simulate timeout by sleeping beyond the client timeout
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(validFlightIntentJSON))
	})
	defer srv.Close()

	// Create a context that times out before the handler responds
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail with context deadline exceeded")
	}
}

// =============================================================================
// Task 4.5.4 — HTTP error handling
// =============================================================================

func TestParse_HTTP500_ReturnsError(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal server error"}}`))
	})
	defer srv.Close()

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail on HTTP 500")
	}
}

func TestParse_HTTP401_ReturnsError(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	})
	defer srv.Close()

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail on HTTP 401")
	}
}

func TestParse_EmptyResponse_ReturnsError(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No choices in response
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-123",
			"choices": []map[string]interface{}{},
		})
	})
	defer srv.Close()

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail on empty choices")
	}
}

// =============================================================================
// Task 4.5.5 — System prompt includes correct JSON schema
// =============================================================================

func TestSystemPrompt_ContainsRequiredFields(t *testing.T) {
	required := []string{
		`"type"`,
		`"confidence"`,
		`"missing_fields"`,
		`"follow_up"`,
		`"flight_params"`,
		`"hotel_params"`,
		`"departure"`,
		`"arrival"`,
		`"outbound_date"`,
		`"return_date"`,
		`"adults"`,
		`"trip_type"`,
		`"travel_class"`,
		`"stops"`,
		`"query"`,
		`"check_in_date"`,
		`"check_out_date"`,
		`"rating"`,
		`"hotel_classes"`,
		`"amenities"`,
		`"free_cancellation"`,
		`"vacation_rentals"`,
		`"max_price"`,
		`YYYY-MM-DD`,
	}

	for _, field := range required {
		if !strings.Contains(systemPrompt, field) {
			t.Errorf("System prompt is missing %q", field)
		}
	}

	// Values appear within pipe-separated enum strings
	valueChecks := []string{
		`flights|hotels|both|ambiguous|incomplete`,
		`round_trip|one_way`,
		`economy|premium_economy|business|first`,
	}
	for _, val := range valueChecks {
		if !strings.Contains(systemPrompt, val) {
			t.Errorf("System prompt is missing value string %q", val)
		}
	}

	// Check IATA appears (in city mapping rules)
	if !strings.Contains(systemPrompt, "IATA") {
		t.Error("System prompt is missing IATA reference")
	}
}

// =============================================================================
// Task 4.5.6 — History integration
// =============================================================================

func TestParse_WithConversationHistory(t *testing.T) {
	var receivedMessages []chatMessage
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		// Capture messages sent to the API
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedMessages = req.Messages

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(validFlightIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	history := []domain.ConversationMessage{
		{Role: "user", Content: "vuelo a Barcelona", Timestamp: time.Now()},
		{Role: "assistant", Content: "¿Qué fecha?", Timestamp: time.Now()},
	}

	_, err := adapter.Parse(ctx, "en agosto", history, "es")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have: system + history[0] + history[1] + current message = 4 messages
	if len(receivedMessages) != 4 {
		t.Errorf("Expected 4 messages (system + 2 history + 1 current), got %d", len(receivedMessages))
	}
	if receivedMessages[0].Role != "system" {
		t.Errorf("First message should be system, got %q", receivedMessages[0].Role)
	}
	if receivedMessages[1].Role != "user" && receivedMessages[1].Content != "vuelo a Barcelona" {
		t.Errorf("Second message should be user history, got role=%q content=%q", receivedMessages[1].Role, receivedMessages[1].Content)
	}
	if receivedMessages[3].Role != "user" && receivedMessages[3].Content != "en agosto" {
		t.Errorf("Last message should be current user message, got role=%q content=%q", receivedMessages[3].Role, receivedMessages[3].Content)
	}
}

// =============================================================================
// RepairJSON unit tests
// =============================================================================

func TestRepairJSON_MarkdownCodeBlock(t *testing.T) {
	input := "```json\n{\"type\": \"flights\"}\n```"
	result := repairJSON(input)
	if !strings.Contains(result, `"type"`) {
		t.Errorf("repairJSON should extract from markdown block, got: %s", result)
	}
	if strings.Contains(result, "```") {
		t.Error("repairJSON should remove markdown fences")
	}
}

func TestRepairJSON_TextAround(t *testing.T) {
	input := "Here is the JSON:\n{\"type\": \"flights\"}\nHope this helps!"
	result := repairJSON(input)
	if result != `{"type": "flights"}` {
		t.Errorf("repairJSON should strip surrounding text, got: %s", result)
	}
}

func TestRepairJSON_TrailingCommas(t *testing.T) {
	input := `{"type": "flights",}`
	result := repairJSON(input)
	if strings.Contains(result, ",}") {
		t.Errorf("repairJSON should remove trailing commas in braces, got: %s", result)
	}
}

func TestRepairJSON_TrailingCommasInArray(t *testing.T) {
	input := `{"amenities": ["wifi", "pool",]}`
	result := repairJSON(input)
	if strings.Contains(result, ",]") {
		t.Errorf("repairJSON should remove trailing commas in arrays, got: %s", result)
	}
}

// =============================================================================
// fixUnquotedStrings unit tests
// =============================================================================

func TestFixUnquotedStrings_BareEnumValue(t *testing.T) {
	input := `{"type": flights, "confidence": 0.9}`
	result := fixUnquotedStrings(input)
	if !strings.Contains(result, `"type": "flights"`) {
		t.Errorf("should quote 'flights', got: %s", result)
	}
	if strings.Contains(result, `flights,`) {
		t.Errorf("should not leave bare 'flights' unquoted, got: %s", result)
	}
}

func TestFixUnquotedStrings_IATACodes(t *testing.T) {
	input := `{"departure": EZE, "arrival": MAD}`
	result := fixUnquotedStrings(input)
	if !strings.Contains(result, `"departure": "EZE"`) {
		t.Errorf("should quote 'EZE', got: %s", result)
	}
	if !strings.Contains(result, `"arrival": "MAD"`) {
		t.Errorf("should quote 'MAD', got: %s", result)
	}
}

func TestFixUnquotedStrings_SkipsLiterals(t *testing.T) {
	input := `{"free_cancellation": true, "max_price": null, "active": false}`
	result := fixUnquotedStrings(input)
	// true, false, null should remain unquoted
	if !strings.Contains(result, `"free_cancellation": true`) {
		t.Errorf("should keep 'true' unquoted, got: %s", result)
	}
	if !strings.Contains(result, `"max_price": null`) {
		t.Errorf("should keep 'null' unquoted, got: %s", result)
	}
	if !strings.Contains(result, `"active": false`) {
		t.Errorf("should keep 'false' unquoted, got: %s", result)
	}
}

func TestFixUnquotedStrings_NumbersUntouched(t *testing.T) {
	input := `{"adults": 2, "rating": 8, "max_price": 200}`
	result := fixUnquotedStrings(input)
	if !strings.Contains(result, `"adults": 2`) {
		t.Errorf("should leave number 2 alone, got: %s", result)
	}
	if !strings.Contains(result, `"rating": 8`) {
		t.Errorf("should leave number 8 alone, got: %s", result)
	}
}

// =============================================================================
// extractMinimumIntent unit tests
// =============================================================================

func TestExtractMinimumIntent_FlightsDowngradesToIncomplete(t *testing.T) {
	raw := `some garbage {"type": "flights"} more garbage`
	intent, err := extractMinimumIntent(raw)
	if err != nil {
		t.Fatalf("extractMinimumIntent should succeed, got: %v", err)
	}
	if intent.Type != "incomplete" {
		t.Errorf("expected type 'incomplete' (downgraded from 'flights'), got %q", intent.Type)
	}
}

func TestExtractMinimumIntent_KeepsAmbiguous(t *testing.T) {
	raw := `{"type": "ambiguous", "follow_up": "¿Qué buscás?"}`
	intent, err := extractMinimumIntent(raw)
	if err != nil {
		t.Fatalf("extractMinimumIntent should succeed, got: %v", err)
	}
	if intent.Type != "ambiguous" {
		t.Errorf("expected type 'ambiguous', got %q", intent.Type)
	}
	if intent.FollowUp != "¿Qué buscás?" {
		t.Errorf("expected follow_up '¿Qué buscás?', got %q", intent.FollowUp)
	}
}

func TestExtractMinimumIntent_KeepsIncomplete(t *testing.T) {
	raw := `{"type": "incomplete", "follow_up": "¿Qué fecha?"}`
	intent, err := extractMinimumIntent(raw)
	if err != nil {
		t.Fatalf("extractMinimumIntent should succeed, got: %v", err)
	}
	if intent.Type != "incomplete" {
		t.Errorf("expected type 'incomplete', got %q", intent.Type)
	}
}

func TestExtractMinimumIntent_NoTypeFieldReturnsError(t *testing.T) {
	raw := `{"something": "else"}`
	_, err := extractMinimumIntent(raw)
	if err == nil {
		t.Fatal("expected error when no type field is present")
	}
}

// =============================================================================
// Progressive repair: unquoted strings → successful parse
// =============================================================================

func TestParse_UnquotedStrings_RepairedAndSucceeds(t *testing.T) {
	unquotedJSON := `{
	"type": flights,
	"confidence": 0.9,
	"flight_params": {
		"departure": EZE,
		"arrival": MAD,
		"outbound_date": "2026-06-15",
		"trip_type": round_trip,
		"adults": 1
	}
}`

	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(unquotedJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err != nil {
		t.Fatalf("Parse should succeed after fixing unquoted strings, got: %v", err)
	}
	if intent.Type != "flights" {
		t.Errorf("Type = %q, want 'flights'", intent.Type)
	}
	if intent.FlightParams.Departure != "EZE" {
		t.Errorf("Departure = %q, want 'EZE'", intent.FlightParams.Departure)
	}
	if intent.FlightParams.TripType != "round_trip" {
		t.Errorf("TripType = %q, want 'round_trip'", intent.FlightParams.TripType)
	}
}

// =============================================================================
// Fallback extraction: completely malformed JSON → minimum intent
// =============================================================================

func TestParse_FallbackExtraction_SalvagesTypeField(t *testing.T) {
	// JSON with an unquoted key (bad) that makes json.Unmarshal fail even
	// after all repair steps. The type and follow_up fields are still extractable.
	brokenJSON := `{"type": "incomplete", "follow_up": "¿Cuándo querés viajar?", bad: value}`

	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIChatResponse(brokenJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "quiero viajar", nil, "es")

	if err != nil {
		t.Fatalf("Parse should succeed via fallback extraction, got: %v", err)
	}
	// incomplete is preserved as-is
	if intent.Type != "incomplete" {
		t.Errorf("Type = %q, want 'incomplete'", intent.Type)
	}
	if intent.FollowUp != "¿Cuándo querés viajar?" {
		t.Errorf("FollowUp = %q, want '¿Cuándo querés viajar?'", intent.FollowUp)
	}
}

// =============================================================================
// Task 1.2 — Tool support in client.go
// =============================================================================

func TestChatCompletionRequest_MarshalsWithTools(t *testing.T) {
	// RED: ToolDef type does not exist yet in deepseek package.
	req := chatCompletionRequest{
		Model: "deepseek-chat",
		Messages: []chatMessage{
			{Role: "user", Content: "busco hoteles en Barcelona"},
		},
		Temperature: 0.7,
		MaxTokens:   4096,
		Tools: []ToolDef{
			{
				Type: "function",
				Function: ToolFunction{
					Name:        "search_hotels",
					Description: "Busca hoteles",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
				},
			},
		},
		Stream: true,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify tools array is present
	tools, ok := decoded["tools"].([]interface{})
	if !ok {
		t.Fatal("tools field missing or not an array in marshalled JSON")
	}
	if len(tools) != 1 {
		t.Fatalf("tools length = %d, want 1", len(tools))
	}

	tool0 := tools[0].(map[string]interface{})
	if tool0["type"] != "function" {
		t.Errorf("tools[0].type = %v, want 'function'", tool0["type"])
	}

	fn := tool0["function"].(map[string]interface{})
	if fn["name"] != "search_hotels" {
		t.Errorf("tools[0].function.name = %v, want 'search_hotels'", fn["name"])
	}
}

func TestChatCompletionRequest_ToolChoiceOmitzero(t *testing.T) {
	// When ToolChoice is empty (zero value), it should NOT appear in JSON
	req := chatCompletionRequest{
		Model:      "deepseek-chat",
		Messages:   []chatMessage{{Role: "user", Content: "hola"}},
		MaxTokens:  100,
		Stream:     false,
		// ToolChoice intentionally left empty
	}

	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(payload, &decoded)

	if _, exists := decoded["tool_choice"]; exists {
		t.Error("tool_choice should be omitted when empty (omitzero)")
	}
}

func TestChatCompletionRequest_ToolsOmittedWhenEmpty(t *testing.T) {
	// Tools slice nil → should be omitted from JSON
	req := chatCompletionRequest{
		Model:    "deepseek-chat",
		Messages: []chatMessage{{Role: "user", Content: "hola"}},
		MaxTokens: 100,
		Stream:    false,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(payload, &decoded)

	if _, exists := decoded["tools"]; exists {
		t.Error("tools should be omitted when nil/empty")
	}
}

// =============================================================================
// Task 1.2 — SSE tool_call delta parsing
// =============================================================================

func TestSSEStream_ToolCallDeltas(t *testing.T) {
	// Simulate a tool_call SSE chunk
	toolCallChunk := `{
		"choices": [{
			"index": 0,
			"delta": {
				"tool_calls": [{
					"index": 0,
					"id": "call_abc123",
					"function": {
						"name": "search_hotels",
						"arguments": "{\"query\":"
					}
				}]
			}
		}]
	}`

	var chunk struct {
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}

	// RED: The ToolCalls field in the anonymous struct should be recognized
	if err := json.Unmarshal([]byte(toolCallChunk), &chunk); err != nil {
		t.Fatalf("unmarshal tool_call chunk failed: %v", err)
	}

	if len(chunk.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}

	tc := chunk.Choices[0].Delta.ToolCalls
	if len(tc) == 0 {
		t.Fatal("expected tool_calls in delta")
	}
	if tc[0].ID != "call_abc123" {
		t.Errorf("tool_call id = %q, want 'call_abc123'", tc[0].ID)
	}
	if tc[0].Function.Name != "search_hotels" {
		t.Errorf("tool_call function name = %q, want 'search_hotels'", tc[0].Function.Name)
	}
}

// =============================================================================
// Task 1.3 — ChatWithTools (streaming tool calling)
// =============================================================================

// sseToolCallStream returns a handler that simulates a DeepSeek SSE stream
// with text chunks followed by tool_call deltas ending with finish_reason:"tool_calls".
func sseToolCallStream(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}

		// Text chunk before tool call
		chunks := []string{
			`{"choices":[{"index":0,"delta":{"content":"Voy a buscar "}}]}`,
			`{"choices":[{"index":0,"delta":{"content":"hoteles en Barcelona."}}]}`,
			// Tool call delta: function name
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_test001","type":"function","function":{"name":"search_hotels","arguments":""}}]}}]}`,
			// Tool call delta: arguments part 1
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":\"Barcelona"}}]}}]}`,
			// Tool call delta: arguments part 2
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":", España\",\"check_in_date\":\"2026-07-01\",\"check_out_date\":\"2026-07-05\"}"}}]}}]}`,
			// Finish reason: tool_calls
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		}

		for _, chunk := range chunks {
			_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}
}

func TestChatWithTools_ExtractsToolCalls(t *testing.T) {
	// RED: ChatWithTools does not exist yet on Adapter
	adapter, srv := newTestAdapter(sseToolCallStream(t))
	defer srv.Close()

	ctx := t.Context()
	messages := []chatMessage{
		{Role: "system", Content: "Eres un asistente de viajes."},
		{Role: "user", Content: "busco hoteles en Barcelona en julio"},
	}
	tools := []ToolDef{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "search_hotels",
				Description: "Busca hoteles en un destino",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
			},
		},
	}

	result, err := adapter.ChatWithTools(ctx, messages, tools)
	if err != nil {
		t.Fatalf("ChatWithTools failed: %v", err)
	}

	if result.AssistantMessage == "" {
		t.Error("AssistantMessage should not be empty")
	}
	if !containsString(result.AssistantMessage, "Voy a buscar") {
		t.Errorf("AssistantMessage should contain text chunks, got: %s", result.AssistantMessage)
	}

	// Should have exactly one tool call
	if len(result.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(result.ToolCalls))
	}

	tc := result.ToolCalls[0]
	if tc.ID != "call_test001" {
		t.Errorf("ToolCall ID = %q, want 'call_test001'", tc.ID)
	}
	if tc.Name != "search_hotels" {
		t.Errorf("ToolCall Name = %q, want 'search_hotels'", tc.Name)
	}
	if tc.Arguments == nil {
		t.Fatal("ToolCall Arguments should not be nil")
	}
	if tc.Arguments["query"] != "Barcelona, España" {
		t.Errorf("Arguments[query] = %v, want 'Barcelona, España'", tc.Arguments["query"])
	}
	if tc.Arguments["check_in_date"] != "2026-07-01" {
		t.Errorf("Arguments[check_in_date] = %v, want '2026-07-01'", tc.Arguments["check_in_date"])
	}
}

func TestChatWithTools_NoToolCalls(t *testing.T) {
	// When AI responds with text only (no tool calls), ToolCalls should be empty
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		chunks := []string{
			`{"choices":[{"index":0,"delta":{"content":"Hola, ¿en qué puedo ayudarte?"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, chunk := range chunks {
			_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	})
	defer srv.Close()

	ctx := t.Context()
	messages := []chatMessage{
		{Role: "user", Content: "hola"},
	}
	tools := []ToolDef{}

	result, err := adapter.ChatWithTools(ctx, messages, tools)
	if err != nil {
		t.Fatalf("ChatWithTools failed: %v", err)
	}

	if len(result.ToolCalls) != 0 {
		t.Errorf("ToolCalls should be empty for text-only response, got %d", len(result.ToolCalls))
	}
}

func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && indexOfSubstring(s, substr) >= 0)
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
