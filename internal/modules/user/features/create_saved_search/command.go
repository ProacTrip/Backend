// Comando para POST /v1/user/saved-searches.
// Acepta cualquier estructura JSON válida como parámetros de búsqueda:
//   - Flight: {origin, destination, departure_date, return_date, passengers, cabin_class}
//   - Hotel: {destination, check_in, check_out, guests, rooms}
//   - AI: {message, conversation_id, language, context}
//   - Both: {origin, destination, dates, passengers, hotel_prefs}
package create_saved_search

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

var validSearchTypes = map[string]bool{
	"flight": true,
	"hotel":  true,
	"ai":     true,
	"both":   true,
}

// Command contiene los campos para crear una búsqueda guardada.
type Command struct {
	UserID       string           `json:"-"`                        // Del token, nunca del body
	Name         *string          `json:"name,omitzero"`            // Opcional
	Parameters   json.RawMessage  `json:"parameters"`               // Requerido — cualquier JSON válido
	Filters      json.RawMessage  `json:"filters,omitzero"`         // Opcional
	SearchType   *string          `json:"search_type,omitzero"`     // Opcional — flight|hotel|ai|both
	AlertEnabled *bool            `json:"alert_enabled,omitzero"`   // Opcional, default false
}

// Validate verifica que UserID sea un UUID válido y parameters no esté vacío.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if len(c.Parameters) == 0 {
		return errors.New("parameters es requerido")
	}
	if !json.Valid(c.Parameters) {
		return errors.New("parameters no es JSON válido")
	}
	if len(c.Filters) > 0 && !json.Valid(c.Filters) {
		return errors.New("filters no es JSON válido")
	}
	if c.SearchType != nil && *c.SearchType != "" && !validSearchTypes[*c.SearchType] {
		return errors.New("search_type inválido. Valores permitidos: flight, hotel, ai, both")
	}
	return nil
}
