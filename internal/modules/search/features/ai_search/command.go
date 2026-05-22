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
type Command struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id,omitzero"`
	Stream         bool   `json:"stream,omitzero"` // true → SSE streaming response

	// SearchModeHint permite al cliente sugerir el modo de búsqueda.
	// Valores válidos: "discovery", "exact", "" (vacío = automático).
	// Si se especifica un valor inválido, Validate() retorna error.
	SearchModeHint string `json:"search_mode,omitzero"`

	// Location/env context from frontend (forwarded from /v1/environment).
	Lat         float64 `json:"lat,omitzero"`
	Lng         float64 `json:"lng,omitzero"`
	Timezone    string  `json:"timezone,omitzero"`
	CountryCode string  `json:"country_code,omitzero"`

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

	// Validar SearchModeHint si está presente
	if c.SearchModeHint != "" {
		switch SearchMode(c.SearchModeHint) {
		case SearchModeDiscovery, SearchModeExact:
			// válido
		default:
			return fmt.Errorf("%w: search_mode debe ser 'discovery' o 'exact', recibido '%s'",
				domain.ErrInvalidParameterRange, c.SearchModeHint)
		}
	}

	return nil
}
