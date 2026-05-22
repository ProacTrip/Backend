// DTO de entrada para AI search — comando de lenguaje natural.
// Valida que el mensaje no esté vacío.
package ai_search

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	searchshared "github.com/ProacTrip/Backend/internal/modules/search/shared"
)

// Command is the input DTO for AI-powered unified search.
// The user sends a natural language message; the AI interprets it
// and the UseCase orchestrates flight and/or hotel searches.
//
// Backend resolves location (from c.RealIP() → env:{ip} cache),
// currency/language (from UserProfilePort), and AI decides search
// mode via tool calling — none of these are client-provided anymore.
type Command struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id,omitzero"`
	Stream         bool   `json:"stream,omitzero"` // true → SSE streaming response

	// Resolved search defaults — populated by the handler after calling
	// shared.ResolveSearchDefaults(). Not part of the JSON API.
	GL       string `json:"-"`
	HL       string `json:"-"`
	Currency string `json:"-"`
	ClientIP string `json:"-"`
}

// Validate checks that the command fields are valid.
// Returns a domain error wrapping ErrMissingRequiredField if message is empty.
func (c *Command) Validate() error {
	if c.Message == "" {
		return fmt.Errorf("%w: message", domain.ErrMissingRequiredField)
	}

	// Trim spaces and check again — whitespace-only is invalid
	trimmed := searchshared.TrimSpaces(c.Message)
	if trimmed == "" {
		return fmt.Errorf("%w: message", domain.ErrMissingRequiredField)
	}

	return nil
}
