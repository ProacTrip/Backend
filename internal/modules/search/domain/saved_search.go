// Puerto para acceder a búsquedas guardadas desde el módulo user.
// El módulo search consume búsquedas guardadas sin acoplarse al módulo user.
package domain

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// SavedSearchProvider provides access to saved searches from the user module.
type SavedSearchProvider interface {
	GetByID(ctx context.Context, searchID uuid.UUID) (*SavedSearchData, error)
}

// SavedSearchData is the subset of a saved search needed by the search module.
type SavedSearchData struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	Name              string
	Parameters        json.RawMessage
	Filters           json.RawMessage
	SearchType        string
	ParametersVersion int
}
