// DTO de respuesta para execute_saved_search.
// Devuelve resultados agrupados por tipo de búsqueda.
package execute_saved_search

import (
	"encoding/json"
)

// Response es la respuesta unificada para POST /v1/search/execute_saved.
type Response struct {
	SearchType string          `json:"search_type"`
	SearchID   string          `json:"search_id"`
	Results    Results         `json:"results"`
}

// Results contiene los resultados anidados por tipo de búsqueda.
type Results struct {
	Flights    json.RawMessage `json:"flights,omitzero"`
	Hotels     json.RawMessage `json:"hotels,omitzero"`
	AIResponse json.RawMessage `json:"ai_response,omitzero"`
	FlightsError string       `json:"flights_error,omitzero"`
	HotelsError  string       `json:"hotels_error,omitzero"`
}
