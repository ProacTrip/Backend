// Tests for Ollama adapter — mock HTTP server with Ollama native API,
// JSON repair, retry, interface check.
// Ollama requires more aggressive JSON repair than DeepSeek (local models are less reliable).
package ollama

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
	client := NewClient(5*time.Second,
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	return NewAdapter(client), srv
}

// validFlightIntentJSON is a valid flights intent.
const validFlightIntentJSON = `{
	"type": "flights",
	"confidence": 0.92,
	"flight_params": {
		"departure": "EZE",
		"arrival": "MAD",
		"outbound_date": "2026-07-10",
		"adults": 1
	}
}`

// validAmbiguousIntentJSON is a valid ambiguous intent.
const validAmbiguousIntentJSON = `{
	"type": "ambiguous",
	"confidence": 0.45,
	"missing_fields": ["intent_type"],
	"follow_up": "¿Buscás vuelos, hoteles, o ambos?",
	"flight_params": null,
	"hotel_params": null
}`

// ollamaNativeChatResponse wraps JSON content in the Ollama native API response format.
func ollamaNativeChatResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"model": "llama3.2",
		"message": map[string]interface{}{
			"role":    "assistant",
			"content": content,
		},
		"done":        true,
		"done_reason": "stop",
	}
}

// =============================================================================
// Task 4.5.1 — Valid JSON response → correct TravelIntent
// =============================================================================

func TestParse_ValidFlightIntent(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request is Ollama native format
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(validFlightIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo de Buenos Aires a Madrid en julio", nil, "es")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if intent.Type != "flights" {
		t.Errorf("Type = %q, want 'flights'", intent.Type)
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
	if intent.FlightParams.OutboundDate != "2026-07-10" {
		t.Errorf("OutboundDate = %q, want '2026-07-10'", intent.FlightParams.OutboundDate)
	}
}

func TestParse_ValidAmbiguousIntent(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(validAmbiguousIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "viaje", nil, "es")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if intent.Type != "ambiguous" {
		t.Errorf("Type = %q, want 'ambiguous'", intent.Type)
	}
	if len(intent.MissingFields) != 1 {
		t.Errorf("MissingFields len = %d, want 1", len(intent.MissingFields))
	}
	if intent.FollowUp == "" {
		t.Error("FollowUp should not be empty")
	}
}

// =============================================================================
// Task 4.5.2 — Malformed JSON → repair + retry (max 3 for Ollama)
// =============================================================================

func TestParse_MarkdownCodeBlock_Repaired(t *testing.T) {
	// Markdown code block is repaired by repairJSON in-call — no retry needed
	callCount := 0
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(
			"```json\n" + validFlightIntentJSON + "\n```",
		))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err != nil {
		t.Fatalf("Parse should succeed after repair: %v", err)
	}
	if intent.Type != "flights" {
		t.Errorf("Type = %q, want 'flights'", intent.Type)
	}
	// repairJSON extracts from markdown → parsed on first attempt
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestParse_TruncatedJSON_Repaired(t *testing.T) {
	// Truncated output is repaired by fixMissingClosingBraces in-call — no retry needed
	callCount := 0
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(
			`{"type": "flights", "confidence": 0.9, "flight_params": {"departure": "EZE", "arrival": "MAD"`,
		))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err != nil {
		t.Fatalf("Parse should succeed after fixing truncated JSON: %v", err)
	}
	if intent.Type != "flights" {
		t.Errorf("Type = %q, want 'flights'", intent.Type)
	}
	// fixMissingClosingBraces repairs on first attempt
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestParse_MalformedJSON_ExhaustsRetries(t *testing.T) {
	callCount := 0
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse("completely invalid {{{["))
	})
	defer srv.Close()

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail after exhausting 3 retries")
	}
	if !strings.Contains(err.Error(), "AI_PARSE_FAILURE") {
		t.Errorf("Expected AI_PARSE_FAILURE in error, got: %v", err)
	}
	// maxRetries = 3, so 1 initial + 3 retries = 4 calls
	if callCount != 4 {
		t.Errorf("Expected 4 calls (1 initial + 3 retries), got %d", callCount)
	}
}

func TestParse_TextBeforeJSON_Repaired(t *testing.T) {
	// Ollama often outputs explanatory text before JSON
	textBeforeJSON := "Aquí tienes la interpretación de tu consulta:\n\n" + validFlightIntentJSON + "\n\n¿Necesitas algo más?"

	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(textBeforeJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err != nil {
		t.Fatalf("Parse should extract JSON from surrounding text: %v", err)
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
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(validFlightIntentJSON))
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail with context deadline exceeded")
	}
}

// =============================================================================
// Task 4.5.4 — HTTP error handling (Ollama native API errors)
// =============================================================================

func TestParse_HTTP500_ReturnsError(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ollamaErrorResponse{Error: "server error"})
	})
	defer srv.Close()

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail on HTTP 500")
	}
}

func TestParse_OllamaDown_ReturnsError(t *testing.T) {
	// Server that refuses connections
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately
		panic("simulated Ollama crash")
	})
	srv.Close() // close the server to simulate Ollama being down

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail when Ollama is unreachable")
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
}

func TestSystemPrompt_InstructsJSONOnly(t *testing.T) {
	phrases := []string{
		"return ONLY this JSON structure",
		"OUTPUT FORMAT",
		"RULES:",
	}

	for _, phrase := range phrases {
		if !strings.Contains(systemPrompt, phrase) {
			t.Errorf("System prompt is missing instruction: %q", phrase)
		}
	}
}

// =============================================================================
// Task 4.5.6 — Ollama-specific: missing closing braces repair
// =============================================================================

func TestFixMissingClosingBraces_Balanced(t *testing.T) {
	input := `{"type": "flights", "confidence": 0.9}`
	result := fixMissingClosingBraces(input)
	if result != input {
		t.Errorf("Balanced JSON should not change, got: %s", result)
	}
}

func TestFixMissingClosingBraces_Unbalanced(t *testing.T) {
	input := `{"type": "flights", "flight_params": {"departure": "EZE"`
	result := fixMissingClosingBraces(input)
	if !strings.HasSuffix(result, "}}") {
		t.Errorf("Should append missing }} to close braces, got: %s", result)
	}
}

func TestFixMissingClosingBraces_NestedArrays(t *testing.T) {
	input := `{"amenities": ["wifi", "pool"`
	result := fixMissingClosingBraces(input)
	if !strings.HasSuffix(result, "]}") {
		t.Errorf("Should append missing ]} to close array and brace, got: %s", result)
	}
}

// =============================================================================
// repairJSON unit tests (Ollama-specific edge cases)
// =============================================================================

func TestRepairJSON_MarkdownCodeBlock(t *testing.T) {
	input := "```json\n{\"type\": \"hotels\"}\n```"
	result := repairJSON(input)
	if !strings.Contains(result, `"type"`) {
		t.Errorf("repairJSON should extract from markdown block, got: %s", result)
	}
	if strings.Contains(result, "```") {
		t.Error("repairJSON should remove markdown fences")
	}
}

func TestRepairJSON_NoCodeBlock_JustJSON(t *testing.T) {
	input := `{"type": "flights"}`
	result := repairJSON(input)
	if result != `{"type": "flights"}` {
		t.Errorf("repairJSON should return clean JSON, got: %s", result)
	}
}

func TestRepairJSON_TrailingCommas(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"trailing comma in object", `{"type": "flights",}`},
		{"trailing comma in array", `{"amenities": ["wifi", "pool",]}`},
		{"both trailing commas", `{"type": "flights", "items": ["a", "b",],}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := repairJSON(tc.input)
			if strings.Contains(result, ",]") {
				t.Errorf("Should remove ,] from: %s", result)
			}
			if strings.Contains(result, ",}") {
				t.Errorf("Should remove ,} from: %s", result)
			}
		})
	}
}

// =============================================================================
// History integration — verifies Ollama native API message format
// =============================================================================

func TestParse_WithConversationHistory(t *testing.T) {
	var receivedMessages []chatMessage
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		// Verify request is Ollama native format
		var req ollamaChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedMessages = req.Messages

		// Verify the request has the right structure
		if req.Model == "" {
			t.Error("Ollama native request must include model field")
		}
		if req.Stream {
			t.Error("Ollama requests should have stream=false for non-streaming mode")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(validFlightIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	history := []domain.ConversationMessage{
		{Role: "user", Content: "vuelo a Madrid en julio", Timestamp: time.Now()},
		{Role: "assistant", Content: "¿Ida y vuelta o solo ida?", Timestamp: time.Now()},
	}

	_, err := adapter.Parse(ctx, "ida y vuelta", history, "es")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(receivedMessages) != 4 {
		t.Errorf("Expected 4 messages (system + 2 history + user), got %d", len(receivedMessages))
	}
	if receivedMessages[0].Role != "system" {
		t.Errorf("First message should be system, got %q", receivedMessages[0].Role)
	}
	if receivedMessages[3].Content != "ida y vuelta" {
		t.Errorf("Last message should be current user message, got %q", receivedMessages[3].Content)
	}
}

// =============================================================================
// Ollama native API endpoint verification
// =============================================================================

func TestClient_UsesNativeAPIEndpoint(t *testing.T) {
	var calledEndpoint string

	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		calledEndpoint = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(validFlightIntentJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Ollama native API uses /api/chat, NOT /v1/chat/completions
	if calledEndpoint != "/api/chat" {
		t.Errorf("expected endpoint /api/chat (Ollama native), got %s", calledEndpoint)
	}
}

// =============================================================================
// Ollama native API response format: empty content handling
// =============================================================================

func TestParse_OllamaEmptyContent_ReturnsError(t *testing.T) {
	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaChatResponse{
			Model: "llama3.2",
			Message: chatMessage{
				Role:    "assistant",
				Content: "", // empty — model may refuse to answer
			},
			Done:       true,
			DoneReason: "load",
		})
	})
	defer srv.Close()

	ctx := t.Context()
	_, err := adapter.Parse(ctx, "vuelo a Madrid", nil, "es")

	if err == nil {
		t.Fatal("Parse should fail when Ollama returns empty content")
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
		t.Errorf("expected follow_up, got %q", intent.FollowUp)
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
	"confidence": 0.85,
	"flight_params": {
		"departure": EZE,
		"arrival": MAD,
		"outbound_date": "2026-06-15",
		"trip_type": one_way,
		"adults": 1
	}
}`

	adapter, srv := newTestAdapter(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(unquotedJSON))
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
		json.NewEncoder(w).Encode(ollamaNativeChatResponse(brokenJSON))
	})
	defer srv.Close()

	ctx := t.Context()
	intent, err := adapter.Parse(ctx, "quiero viajar", nil, "es")

	if err != nil {
		t.Fatalf("Parse should succeed via fallback extraction, got: %v", err)
	}
	if intent.Type != "incomplete" {
		t.Errorf("Type = %q, want 'incomplete'", intent.Type)
	}
	if intent.FollowUp != "¿Cuándo querés viajar?" {
		t.Errorf("FollowUp = %q, want '¿Cuándo querés viajar?'", intent.FollowUp)
	}
}
