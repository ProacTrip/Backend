// Comando para PUT /v1/user/saved-searches/:search_id.
package update_saved_search

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// Command contiene los campos actualizables de una búsqueda guardada.
// Todos opcionales: nil = no actualizar.
type Command struct {
	UserID     string            `json:"-"`
	SearchID   string            `json:"-"`
	Name       *string           `json:"name,omitzero"`       // Opcional
	Parameters *json.RawMessage  `json:"parameters,omitzero"`  // Opcional
	Filters    *json.RawMessage  `json:"filters,omitzero"`    // Opcional
	SearchType *string           `json:"search_type,omitzero"` // Opcional — flight|hotel|ai|both
}

// Validate verifica que los UUIDs sean válidos y los JSONs estén bien formados.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if _, err := uuid.Parse(c.SearchID); err != nil {
		return errors.New("search_id inválido")
	}
	if c.Parameters != nil {
		if len(*c.Parameters) == 0 {
			return errors.New("parameters no puede estar vacío")
		}
		if !json.Valid(*c.Parameters) {
			return errors.New("parameters no es JSON válido")
		}
	}
	if c.Filters != nil && len(*c.Filters) > 0 {
		if !json.Valid(*c.Filters) {
			return errors.New("filters no es JSON válido")
		}
	}
	if c.SearchType != nil && *c.SearchType != "" {
		switch *c.SearchType {
		case "flight", "hotel", "ai", "both":
		default:
			return errors.New("search_type inválido. Valores permitidos: flight, hotel, ai, both")
		}
	}
	return nil
}
