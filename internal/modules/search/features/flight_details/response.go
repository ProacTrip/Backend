// DTO de respuesta para detalles de vuelo.
// Re-exporta el tipo de dominio.
package flight_details

import "github.com/ProacTrip/Backend/internal/modules/search/domain"

// Response is the flight details API response.
// Uses type alias to re-export the domain response directly
// since domain types already have JSON tags.
type Response = domain.FlightDetailsResponse
