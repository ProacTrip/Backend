// Puerto para proveedores externos de búsqueda de vuelos.
// Define la interfaz que deben implementar los adaptadores (ej: SerpAPI).
package domain

import "context"

// =============================================================================
// FlightProvider — Port
// =============================================================================

// FlightProvider is the port for external flight search providers (SerpAPI).
type FlightProvider interface {
	Search(ctx context.Context, req FlightSearchRequest) (*FlightSearchResponse, error)
	GetDetails(ctx context.Context, bookingToken string, adults int, currency string, departureID, arrivalID, outboundDate, returnDate string) (*FlightDetailsResponse, error)
}
