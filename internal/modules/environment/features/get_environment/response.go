package get_environment

import "github.com/ProacTrip/Backend/internal/modules/environment/domain"

// Response = domain.EnvironmentResponse
//
// Mapeo de Compatibilidad SerpAPI (para desarrolladores frontend):
//
//	Campo Environment     Parámetro SerpAPI   Formato
//	location.country_code  gl               ISO 3166-1 alpha-2 (2 chars)
//	location.currency      currency         ISO 4217 (3 chars)
//	location.language      hl               ISO 639-1 (2 chars, desde Accept-Language)
//	location.timezone      tz               IANA timezone (para visualización horaria)
type Response = domain.EnvironmentResponse
