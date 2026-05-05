// Puerto para proveedores externos de búsqueda.
// Define las interfaces que deben implementar los adaptadores (ej: SerpAPI).
// Cada dominio (vuelos, hoteles) tiene su propia interfaz con nombres explícitos
// que evitan colisiones de métodos en adaptadores que implementan ambos.
package domain

import "context"

// =============================================================================
// FlightProvider — Port for flight search providers
// =============================================================================

// FlightProvider is the port for external flight search providers (SerpAPI).
type FlightProvider interface {
	SearchFlights(ctx context.Context, req FlightSearchRequest) (*FlightSearchResponse, error)
	GetFlightDetails(ctx context.Context, req FlightDetailsRequest) (*FlightDetailsResponse, error)
}

// =============================================================================
// HotelProvider — Port for hotel search providers
// =============================================================================

// HotelProvider is the port for external hotel search providers (SerpAPI).
type HotelProvider interface {
	SearchHotels(ctx context.Context, req HotelSearchRequest) (*HotelSearchResponse, error)
	GetHotelDetails(ctx context.Context, req HotelDetailsRequest) (*HotelDetailsResponse, error)
}

// =============================================================================
// AIInterpreter — Port for AI natural language interpretation
// =============================================================================
//
// AIInterpreter is defined in domain/ai.go alongside TravelIntent,
// ConversationState, and FilterCriteria. It is the port for AI-based
// natural language travel interpretation.
//
//	var _ AIInterpreter = (*myAdapter)(nil) // compile-time check
