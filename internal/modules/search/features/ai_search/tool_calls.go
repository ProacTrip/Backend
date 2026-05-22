// Tool definitions para DeepSeek tool calling.
// Define los JSON Schemas de search_hotels y search_flights
// que el AI usa para decidir cuándo y con qué parámetros buscar.
package ai_search

import (
	"encoding/json"
	"fmt"
)

// =============================================================================
// Tool definition types for DeepSeek tool calling
// =============================================================================

// ToolDef represents a function tool definition for DeepSeek tool calling.
type ToolDef struct {
	Type     string       `json:"type"`     // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function with its JSON Schema parameters.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall represents a tool call requested by the AI model.
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"` // "search_hotels" or "search_flights"
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResult wraps the AI response with optional tool calls.
type ToolCallResult struct {
	AssistantMessage string     `json:"assistant_message"`
	ToolCalls        []ToolCall `json:"tool_calls,omitzero"`
}

// ToolResult holds the result of a single tool call execution.
type ToolResult struct {
	CallID      string `json:"call_id"`
	Name        string `json:"name"`
	Destination string `json:"destination,omitzero"` // human-readable destination
	Content     string `json:"content"`              // JSON result for the AI
	Error       error  `json:"-"`
}

// ToolResultJSON is the JSON-serializable wrapper for ToolResult.
type ToolResultJSON struct {
	CallID      string          `json:"call_id"`
	Name        string          `json:"name"`
	Destination string          `json:"destination,omitzero"`
	Content     json.RawMessage `json:"content"`
	Error       string          `json:"error,omitzero"`
}

// ToJSON converts a ToolResult to its JSON-serializable form.
func (tr *ToolResult) ToJSON() ToolResultJSON {
	result := ToolResultJSON{
		CallID:      tr.CallID,
		Name:        tr.Name,
		Destination: tr.Destination,
		Content:     json.RawMessage(tr.Content),
	}
	if tr.Error != nil {
		result.Error = tr.Error.Error()
	}
	return result
}

// ValidateRequired checks that the specified fields are present and non-empty in args.
// Returns a list of missing field names.
func ValidateRequired(args map[string]interface{}, fields ...string) []string {
	var missing []string
	for _, f := range fields {
		v, ok := args[f]
		if !ok || v == nil {
			missing = append(missing, f)
			continue
		}
		if s, isStr := v.(string); isStr && s == "" {
			missing = append(missing, f)
		}
	}
	return missing
}

// ValidateRequiredOrError is like ValidateRequired but returns an error if any fields are missing.
func ValidateRequiredOrError(toolName string, args map[string]interface{}, fields ...string) error {
	missing := ValidateRequired(args, fields...)
	if len(missing) > 0 {
		return fmt.Errorf("tool %s: missing required fields: %v", toolName, missing)
	}
	return nil
}

// =============================================================================
// Tool definitions — JSON Schema for DeepSeek function calling
// =============================================================================

// SearchHotelsToolDef returns the complete search_hotels tool definition
// as a JSON Schema function definition for DeepSeek tool calling.
//
// The AI fills only relevant params (REQ-003). Required: query, check_in_date,
// check_out_date. All others have sensible defaults or are optional.
func SearchHotelsToolDef() ToolDef {
	// language=json
	const params = `{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Destino de búsqueda de hoteles (ciudad, región o país). Ej: 'Barcelona, España', 'Costa de la Luz'"
			},
			"check_in_date": {
				"type": "string",
				"description": "Fecha de entrada en formato YYYY-MM-DD"
			},
			"check_out_date": {
				"type": "string",
				"description": "Fecha de salida en formato YYYY-MM-DD"
			},
			"adults": {
				"type": "integer",
				"description": "Número de adultos",
				"default": 2
			},
			"children": {
				"type": "integer",
				"description": "Número de niños",
				"default": 0
			},
			"children_ages": {
				"type": "array",
				"description": "Edades de los niños (1-17). Debe coincidir en cantidad con children.",
				"items": {"type": "integer"}
			},
			"gl": {
				"type": "string",
				"description": "Código de país de 2 letras para geolocalización (ej: 'es', 'ar', 'us')"
			},
			"hl": {
				"type": "string",
				"description": "Código de idioma de 2 letras (ej: 'es', 'en')"
			},
			"currency": {
				"type": "string",
				"description": "Código de moneda ISO 4217 (ej: 'EUR', 'USD', 'ARS')"
			},
			"min_price": {
				"type": "number",
				"description": "Precio mínimo por noche en la moneda indicada"
			},
			"max_price": {
				"type": "number",
				"description": "Precio máximo por noche en la moneda indicada"
			},
			"sort_by": {
				"type": "integer",
				"description": "Orden: 3=más relevantes, 8=menor precio, 13=distancia al centro, 14=mayor puntuación"
			},
			"rating": {
				"type": "integer",
				"description": "Puntuación mínima: 7=3.5+, 8=4.0+, 9=4.5+",
				"minimum": 7,
				"maximum": 9
			},
			"property_types": {
				"type": "array",
				"description": "Tipos de alojamiento (IDs de SerpAPI)",
				"items": {"type": "integer"}
			},
			"amenities": {
				"type": "array",
				"description": "Comodidades (IDs de SerpAPI: 35=wifi, 4=piscina, 5=gimnasio, 9=parking, 10=spa, etc.)",
				"items": {"type": "integer"}
			},
			"vacation_rentals": {
				"type": "boolean",
				"description": "Incluir alquileres vacacionales",
				"default": false
			},
			"hotel_classes": {
				"type": "array",
				"description": "Clases de hotel (2, 3, 4 o 5 estrellas)",
				"items": {"type": "integer"}
			},
			"brands": {
				"type": "array",
				"description": "Cadenas hoteleras (IDs de SerpAPI)",
				"items": {"type": "integer"}
			},
			"free_cancellation": {
				"type": "boolean",
				"description": "Solo hoteles con cancelación gratuita"
			},
			"special_offers": {
				"type": "boolean",
				"description": "Solo hoteles con ofertas especiales"
			},
			"eco_certified": {
				"type": "boolean",
				"description": "Solo hoteles con certificación ecológica"
			},
			"bedrooms": {
				"type": "integer",
				"description": "Número mínimo de dormitorios"
			},
			"bathrooms": {
				"type": "integer",
				"description": "Número mínimo de baños"
			},
			"page_token": {
				"type": "string",
				"description": "Token de paginación para siguientes resultados"
			}
		},
		"required": ["query", "check_in_date", "check_out_date"]
	}`

	return ToolDef{
		Type: "function",
		Function: ToolFunction{
			Name:        "search_hotels",
			Description: "Busca hoteles y alojamientos en un destino. La AI decide cuándo y con qué filtros llamar. Solo query, check_in_date y check_out_date son obligatorios.",
			Parameters:  json.RawMessage(params),
		},
	}
}

// SearchFlightsToolDef returns the complete search_flights tool definition
// as a JSON Schema function definition for DeepSeek tool calling.
//
// The AI fills only relevant params (REQ-003). Required: trip_type, departure,
// arrival, outbound_date. All others have sensible defaults or are optional.
func SearchFlightsToolDef() ToolDef {
	// language=json
	const params = `{
		"type": "object",
		"properties": {
			"trip_type": {
				"type": "string",
				"description": "Tipo de viaje",
				"enum": ["round_trip", "one_way"]
			},
			"departure": {
				"type": "string",
				"description": "Código IATA del aeropuerto de salida (ej: 'MAD', 'EZE', 'BCN')"
			},
			"arrival": {
				"type": "string",
				"description": "Código IATA del aeropuerto de llegada (ej: 'MAD', 'EZE', 'BCN')"
			},
			"outbound_date": {
				"type": "string",
				"description": "Fecha de ida en formato YYYY-MM-DD"
			},
			"return_date": {
				"type": "string",
				"description": "Fecha de vuelta en formato YYYY-MM-DD. Requerido para round_trip."
			},
			"adults": {
				"type": "integer",
				"description": "Número de adultos",
				"default": 1
			},
			"children": {
				"type": "integer",
				"description": "Número de niños (2-17 años)",
				"default": 0
			},
			"infants_in_seat": {
				"type": "integer",
				"description": "Número de bebés con asiento propio (<2 años)"
			},
			"infants_on_lap": {
				"type": "integer",
				"description": "Número de bebés en el regazo (<2 años)"
			},
			"travel_class": {
				"type": "string",
				"description": "Clase de viaje",
				"enum": ["economy", "premium_economy", "business", "first"]
			},
			"gl": {
				"type": "string",
				"description": "Código de país de 2 letras para geolocalización (ej: 'es', 'ar', 'us')"
			},
			"hl": {
				"type": "string",
				"description": "Código de idioma de 2 letras (ej: 'es', 'en')"
			},
			"currency": {
				"type": "string",
				"description": "Código de moneda ISO 4217 (ej: 'EUR', 'USD', 'ARS')"
			},
			"bags": {
				"type": "integer",
				"description": "Número de maletas facturadas (no puede superar el número de pasajeros)"
			},
			"max_price": {
				"type": "number",
				"description": "Precio máximo por billete en la moneda indicada"
			},
			"sort_by": {
				"type": "string",
				"description": "Criterio de orden",
				"enum": ["top", "price", "departure_time", "arrival_time", "duration", "emissions"]
			},
			"stops": {
				"type": "string",
				"description": "Número de escalas",
				"enum": ["any", "nonstop", "max_1", "max_2"]
			},
			"include_airlines": {
				"type": "array",
				"description": "Aerolíneas a incluir (códigos IATA de 2 letras: 'IB', 'UX', 'AR', etc.)",
				"items": {"type": "string"}
			},
			"exclude_airlines": {
				"type": "array",
				"description": "Aerolíneas a excluir (códigos IATA de 2 letras). Mutuamente excluyente con include_airlines.",
				"items": {"type": "string"}
			},
			"outbound_times": {
				"type": "object",
				"description": "Rango horario de salida. Ej: {\"departure_from\": 6, \"departure_to\": 18}",
				"properties": {
					"departure_from": {"type": "integer", "minimum": 0, "maximum": 23},
					"departure_to": {"type": "integer", "minimum": 0, "maximum": 23},
					"arrival_from": {"type": "integer", "minimum": 0, "maximum": 23},
					"arrival_to": {"type": "integer", "minimum": 0, "maximum": 23}
				}
			},
			"return_times": {
				"type": "object",
				"description": "Rango horario de llegada/vuelta. Misma estructura que outbound_times.",
				"properties": {
					"departure_from": {"type": "integer", "minimum": 0, "maximum": 23},
					"departure_to": {"type": "integer", "minimum": 0, "maximum": 23},
					"arrival_from": {"type": "integer", "minimum": 0, "maximum": 23},
					"arrival_to": {"type": "integer", "minimum": 0, "maximum": 23}
				}
			},
			"emissions_filter": {
				"type": "boolean",
				"description": "Filtrar por vuelos con menores emisiones de CO2"
			},
			"layover_duration": {
				"type": "object",
				"description": "Duración de escala. Ej: {\"min_minutes\": 60, \"max_minutes\": 180}",
				"properties": {
					"min_minutes": {"type": "integer", "minimum": 0},
					"max_minutes": {"type": "integer", "minimum": 0}
				}
			},
			"exclude_connections": {
				"type": "array",
				"description": "Aeropuertos de conexión a excluir (códigos IATA)",
				"items": {"type": "string"}
			},
			"max_duration_minutes": {
				"type": "integer",
				"description": "Duración máxima del vuelo en minutos"
			},
			"cursor": {
				"type": "string",
				"description": "Token de paginación para siguientes resultados"
			},
			"limit": {
				"type": "integer",
				"description": "Número máximo de resultados (default 10, max 100)"
			}
		},
		"required": ["trip_type", "departure", "arrival", "outbound_date"]
	}`

	return ToolDef{
		Type: "function",
		Function: ToolFunction{
			Name:        "search_flights",
			Description: "Busca vuelos entre dos aeropuertos. La AI decide cuándo y con qué filtros llamar. Solo trip_type, departure, arrival y outbound_date son obligatorios.",
			Parameters:  json.RawMessage(params),
		},
	}
}
