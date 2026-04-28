// DTO de respuesta para búsqueda de vuelos.
// Re-exporta el tipo de dominio.
package search_flights

import "github.com/ProacTrip/Backend/internal/modules/search/domain"

// Response is the search flights API response.
// Uses type alias to re-export the domain response directly
// since domain types already have JSON tags.
type Response = domain.FlightSearchResponse
