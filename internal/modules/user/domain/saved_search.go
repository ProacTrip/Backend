// Domain: Búsquedas guardadas del usuario.
// Permite persistir y recibir alertas sobre búsquedas recurrentes.
package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// SavedSearch — Búsqueda guardada con hash de deduplicación
// =============================================================================

// HashService define el puerto de dominio para hashing de contenido.
// Separa la implementación criptográfica (blake3) del dominio.
type HashService interface {
	Hash(data []byte) string
}

// SavedSearch representa una búsqueda guardada por el usuario.
// Alineado con la migración saved_searches.
type SavedSearch struct {
	ID             uuid.UUID       `json:"id"`
	UserID         uuid.UUID       `json:"user_id"`
	Name           *string         `json:"name,omitzero"`
	Parameters     json.RawMessage `json:"parameters"`
	Filters        json.RawMessage `json:"filters,omitzero"`
	SearchHash        string          `json:"search_hash"`
	SearchType        *string         `json:"search_type,omitzero"`
	ParametersVersion int             `json:"parameters_version"` // schema version, default 1
	AlertEnabled      bool            `json:"alert_enabled"`
	LastExecutedAt *time.Time      `json:"last_executed_at,omitzero"`
	ResultCount    *int            `json:"result_count,omitzero"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// NewSavedSearch crea una nueva búsqueda guardada.
// searchHash debe ser pre-calculado por el caso de uso usando HashService.
func NewSavedSearch(userID uuid.UUID, name string, parameters map[string]any, searchHash string) (*SavedSearch, error) {
	now := time.Now()
	paramsJSON, err := json.Marshal(parameters)
	if err != nil {
		return nil, fmt.Errorf("marshal saved search parameters: %w", err)
	}
	return &SavedSearch{
		ID:                uuid.Must(uuid.NewV7()),
		UserID:            userID,
		Name:              &name,
		Parameters:        paramsJSON,
		SearchHash:        searchHash,
		ParametersVersion: 1,
		AlertEnabled:      false,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// ToggleAlert alterna las alertas para esta búsqueda.
func (ss *SavedSearch) ToggleAlert() {
	ss.AlertEnabled = !ss.AlertEnabled
	ss.UpdatedAt = time.Now()
}

// SetFilters establece los filtros de búsqueda.
func (ss *SavedSearch) SetFilters(filters map[string]any) error {
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return fmt.Errorf("marshal saved search filters: %w", err)
	}
	ss.Filters = filtersJSON
	ss.UpdatedAt = time.Now()
	return nil
}


