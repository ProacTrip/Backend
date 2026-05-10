// Comando para POST /v1/user/favorites.
package add_favorite

import (
	"errors"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/google/uuid"
)

// Command contiene los campos para agregar un favorito.
type Command struct {
	UserID     string  `json:"-"`                         // Del token, nunca del body
	EntityID   string  `json:"entity_id"`                 // UUID requerido
	EntityType string  `json:"entity_type"`               // hotel|flight|activity
	Title      string  `json:"title"`                     // Requerido
	Notes      *string `json:"notes,omitzero"`            // Opcional
}

// Validate verifica que los UUIDs sean válidos y los campos requeridos estén presentes.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	if _, err := uuid.Parse(c.EntityID); err != nil {
		return errors.New("entity_id inválido")
	}
	if c.EntityType == "" {
		return errors.New("entity_type es requerido")
	}
	if !domain.IsValidFavoriteEntityType(c.EntityType) {
		return domain.ErrInvalidFavoriteEntityType
	}
	if c.Title == "" {
		return errors.New("title es requerido")
	}
	return nil
}

