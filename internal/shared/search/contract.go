// Contrato compartido para búsquedas guardadas entre módulos user y search.
// Ambos módulos importan desde shared/search — sin acoplamiento circular.
package search

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// SavedSearchProvider es el puerto que el módulo search consume para acceder
// a búsquedas guardadas del módulo user sin acoplarse directamente.
type SavedSearchProvider interface {
	GetByID(ctx context.Context, searchID uuid.UUID) (*SavedSearchData, error)
}

// SavedSearchData es el subconjunto de datos de una búsqueda guardada que
// el módulo search necesita para ejecutar búsquedas recurrentes.
type SavedSearchData struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	Name              string
	Parameters        json.RawMessage
	Filters           json.RawMessage
	SearchType        string
	ParametersVersion int
}
