// DTO de respuesta para GET /v1/user/saved-searches.
package list_saved_searches

// SearchItem representa una búsqueda guardada en la respuesta de listado.
type SearchItem struct {
	ID                string  `json:"id"`
	Name              *string `json:"name,omitzero"`
	Parameters        any     `json:"parameters"`
	Filters           any     `json:"filters,omitzero"`
	SearchType        *string `json:"search_type,omitzero"`
	ParametersVersion int     `json:"parameters_version"`
	AlertEnabled      bool    `json:"alert_enabled"`
	LastExecutedAt    *string `json:"last_executed_at,omitzero"`
	ResultCount       *int    `json:"result_count,omitzero"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

// Response agrupa la lista de búsquedas guardadas del usuario.
type Response struct {
	Searches []SearchItem `json:"searches"`
}
