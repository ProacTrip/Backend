// Tests unitarios para helpers compartidos del módulo search.
// PtrOrEmpty y TrimSpaces — funciones puras, sin dependencias externas.
package shared

import (
	"testing"
)

func TestPtrOrEmpty(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected string
	}{
		{
			name:     "puntero nil devuelve string vacío",
			input:    nil,
			expected: "",
		},
		{
			name:     "puntero a string vacío devuelve string vacío",
			input:    new(""),
			expected: "",
		},
		{
			name:     "puntero a string con valor devuelve el valor",
			input:    new("Buenos Aires"),
			expected: "Buenos Aires",
		},
		{
			name:     "puntero a string con espacios devuelve los espacios",
			input:    new("  con espacios  "),
			expected: "  con espacios  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PtrOrEmpty(tt.input)
			if got != tt.expected {
				t.Errorf("PtrOrEmpty(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTrimSpaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "string vacío devuelve string vacío",
			input:    "",
			expected: "",
		},
		{
			name:     "string sin espacios devuelve igual",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "string con espacios al inicio y final los elimina",
			input:    "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "solo espacios devuelve string vacío",
			input:    "   ",
			expected: "",
		},
		{
			name:     "tabs y newlines también se eliminan",
			input:    "\t\n  valor \n\t",
			expected: "valor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TrimSpaces(tt.input)
			if got != tt.expected {
				t.Errorf("TrimSpaces(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
