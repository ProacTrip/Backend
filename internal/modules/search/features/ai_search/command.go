// DTO de entrada para AI search — comando de lenguaje natural.
// Valida que el mensaje no esté vacío.
package ai_search

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// Command is the input DTO for AI-powered unified search.
// The user sends a natural language message; the AI interprets it
// and the UseCase orchestrates flight and/or hotel searches.
type Command struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id,omitzero"`
	Stream         bool   `json:"stream,omitzero"` // true → SSE streaming response

	// Resolved search defaults — populated by the handler after calling
	// shared.ResolveSearchDefaults(). Not part of the JSON API.
	GL       string `json:"-"`
	HL       string `json:"-"`
	Currency string `json:"-"`

	// ClientIP is the user's IP address, set by the handler for IP-based
	// location detection (anonymous users). Not part of the JSON API.
	ClientIP string `json:"-"`
}

// Validate checks that the command fields are valid.
// Returns a domain error wrapping ErrMissingRequiredField if message is empty.
func (c *Command) Validate() error {
	if c.Message == "" {
		return fmt.Errorf("%w: message", domain.ErrMissingRequiredField)
	}

	// Trim spaces and check again — whitespace-only is invalid
	trimmed := trimSpaces(c.Message)
	if trimmed == "" {
		return fmt.Errorf("%w: message", domain.ErrMissingRequiredField)
	}

	return nil
}

// trimSpaces removes leading and trailing whitespace.
func trimSpaces(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
