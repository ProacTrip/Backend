package ai_search_test

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
)

// =============================================================================
// Command.Validate() tests
// =============================================================================

// TestCommandValidate_ValidMessage verifies that a valid command with a message
// passes validation.
func TestCommandValidate_ValidMessage(t *testing.T) {
	cmd := ai_search.Command{
		Message: "Busco vuelos de Buenos Aires a Madrid la semana que viene",
	}

	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for valid message, got: %v", err)
	}
}

// TestCommandValidate_EmptyMessage verifies that an empty message returns
// a validation error.
func TestCommandValidate_EmptyMessage(t *testing.T) {
	cmd := ai_search.Command{
		Message: "",
	}

	err := cmd.Validate()
	if err == nil {
		t.Error("expected validation error for empty message, got nil")
	}
}

// TestCommandValidate_WithConversationID verifies that a valid message with
// an optional conversation_id also passes validation.
func TestCommandValidate_WithConversationID(t *testing.T) {
	cmd := ai_search.Command{
		Message:        "Quiero hoteles en Barcelona",
		ConversationID: "conv_abc123",
	}

	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for valid message with conversation_id, got: %v", err)
	}
}

// TestCommandValidate_OnlyWhitespace verifies that whitespace-only message
// is treated as empty and returns an error.
func TestCommandValidate_OnlyWhitespace(t *testing.T) {
	cmd := ai_search.Command{
		Message: "   ",
	}

	err := cmd.Validate()
	if err == nil {
		t.Error("expected validation error for whitespace-only message, got nil")
	}
}
