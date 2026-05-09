// Domain: Búsquedas guardadas del usuario.
// Permite persistir y recibir alertas sobre búsquedas recurrentes.
package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"lukechampine.com/blake3"
)

// =============================================================================
// SavedSearch — Búsqueda guardada con hash de deduplicación
// =============================================================================

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
// Calcula automáticamente el hash de los parámetros para deduplicación.
func NewSavedSearch(userID uuid.UUID, name string, parameters map[string]any) (*SavedSearch, error) {
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
		SearchHash:        GenerateSearchHash(parameters),
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

// =============================================================================
// Utilidades
// =============================================================================

// GenerateSearchHash genera un blake3 content hash for deduplication (not cryptographic signature).
// Se usa para detectar búsquedas duplicadas del mismo usuario.
func GenerateSearchHash(params map[string]any) string {
	// Marshal determinista (sorted keys) para hash consistente
	data, err := json.Marshal(params)
	if err != nil {
		// Fallback: usar un hash vacío en caso de error (no debería ocurrir)
		return ""
	}
	h := blake3.Sum256(data)
	return fmt.Sprintf("%x", h)
}
