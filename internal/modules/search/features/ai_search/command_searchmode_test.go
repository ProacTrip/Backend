package ai_search_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
)

// =============================================================================
// Command.Validate() tests — SearchModeHint
// =============================================================================

func TestCommandValidate_ValidSearchModeHint(t *testing.T) {
	tests := []struct {
		name string
		hint string
	}{
		{"discovery", "discovery"},
		{"exact", "exact"},
		{"vacío (omitzero)", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := ai_search.Command{
				Message:        "viaje a la playa",
				SearchModeHint: tc.hint,
			}

			err := cmd.Validate()
			if err != nil {
				t.Errorf("expected no error for valid SearchModeHint=%q, got: %v", tc.hint, err)
			}
		})
	}
}

func TestCommandValidate_InvalidSearchModeHint(t *testing.T) {
	tests := []struct {
		name string
		hint string
	}{
		{"valor desconocido", "unknown_mode"},
		{"vacío explícito", ""}, // vacío es válido
		{"mayúsculas", "DISCOVERY"},
		{"parcial", "disc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := ai_search.Command{
				Message:        "viaje a la playa",
				SearchModeHint: tc.hint,
			}

			err := cmd.Validate()
			if tc.hint == "" {
				// Vacío siempre es válido
				if err != nil {
					t.Errorf("expected no error for empty SearchModeHint, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error for invalid SearchModeHint=%q, got nil", tc.hint)
			}
		})
	}
}

func TestCommandValidate_MessageValidWithModeHint(t *testing.T) {
	// Verifica que la validación general sigue funcionando con SearchModeHint
	cmd := ai_search.Command{
		Message:        "recomiéndame playas",
		SearchModeHint: "discovery",
	}

	err := cmd.Validate()
	if err != nil {
		t.Errorf("expected no error for valid message + mode hint, got: %v", err)
	}

	// Mensaje vacío debería fallar incluso con hint válido
	cmd2 := ai_search.Command{
		Message:        "",
		SearchModeHint: "discovery",
	}
	err = cmd2.Validate()
	if err == nil {
		t.Error("expected error for empty message even with valid hint")
	}
}

func TestCommand_SearchModeHint_OmitZero(t *testing.T) {
	// Verifica que SearchModeHint vacío se omite en JSON
	cmd := ai_search.Command{
		Message: "viaje",
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	if strings.Contains(string(data), "search_mode") {
		t.Errorf("search_mode should be omitted when empty, got: %s", string(data))
	}
}

func TestCommand_SearchModeHint_InJSON(t *testing.T) {
	// Verifica que SearchModeHint se incluye cuando no está vacío
	cmd := ai_search.Command{
		Message:        "viaje",
		SearchModeHint: "discovery",
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if mode, ok := decoded["search_mode"]; !ok {
		t.Error("search_mode should be present when set")
	} else if mode != "discovery" {
		t.Errorf("search_mode = %v, want 'discovery'", mode)
	}
}
