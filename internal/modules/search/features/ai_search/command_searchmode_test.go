package ai_search_test

import (
	"encoding/json"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
)

// =============================================================================
// Command.Validate() tests — basic validation (SearchModeHint removed)
// =============================================================================

func TestCommandValidate_AfterCleanup_ValidMessage(t *testing.T) {
	cmd := ai_search.Command{
		Message: "viaje a la playa",
	}

	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for valid message, got: %v", err)
	}
}

func TestCommandValidate_AfterCleanup_EmptyMessage(t *testing.T) {
	cmd := ai_search.Command{
		Message: "",
	}

	err := cmd.Validate()
	if err == nil {
		t.Error("expected error for empty message, got nil")
	}
}

func TestCommandValidate_WhitespaceOnly(t *testing.T) {
	cmd := ai_search.Command{
		Message: "   \t  \n  ",
	}

	err := cmd.Validate()
	if err == nil {
		t.Error("expected error for whitespace-only message, got nil")
	}
}

func TestCommand_JSON_MarshalOmitZero(t *testing.T) {
	// Verifica que conversation_id vacío y stream=false se omiten en JSON.
	cmd := ai_search.Command{
		Message: "viaje",
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// message should always be present
	if msg, ok := decoded["message"]; !ok {
		t.Error("message field should be present")
	} else if msg != "viaje" {
		t.Errorf("message = %v, want 'viaje'", msg)
	}

	// conversation_id should be omitted when empty
	if _, ok := decoded["conversation_id"]; ok {
		t.Error("conversation_id should be omitted when empty")
	}

	// stream should be omitted when false
	if _, ok := decoded["stream"]; ok {
		t.Error("stream should be omitted when false")
	}

	// search_mode should NEVER be present (removed from API)
	if _, ok := decoded["search_mode"]; ok {
		t.Error("search_mode should not be present (removed from Command)")
	}

	// lat, lng, timezone, country_code should NEVER be present (removed from API)
	if _, ok := decoded["lat"]; ok {
		t.Error("lat should not be present (removed from Command)")
	}
	if _, ok := decoded["lng"]; ok {
		t.Error("lng should not be present (removed from Command)")
	}
	if _, ok := decoded["timezone"]; ok {
		t.Error("timezone should not be present (removed from Command)")
	}
	if _, ok := decoded["country_code"]; ok {
		t.Error("country_code should not be present (removed from Command)")
	}
}

func TestCommand_JSON_WithConversationID(t *testing.T) {
	// Verifica que conversation_id se incluye cuando no está vacío.
	cmd := ai_search.Command{
		Message:        "viaje",
		ConversationID: "conv-abc-123",
		Stream:         true,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if cid, ok := decoded["conversation_id"]; !ok {
		t.Error("conversation_id should be present when set")
	} else if cid != "conv-abc-123" {
		t.Errorf("conversation_id = %v, want 'conv-abc-123'", cid)
	}

	if stream, ok := decoded["stream"]; !ok {
		t.Error("stream should be present when true")
	} else if stream != true {
		t.Errorf("stream = %v, want true", stream)
	}
}
