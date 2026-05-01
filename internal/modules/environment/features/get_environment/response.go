package get_environment

import "github.com/ProacTrip/Backend/internal/modules/environment/domain"

// Response = domain.EnvironmentResponse
//
// SerpAPI Compatibility Mapping (for frontend developers):
//
//	Environment Field   SerpAPI Parameter   Format
//	location.country_code  gl               ISO 3166-1 alpha-2 (2-char)
//	location.currency      currency         ISO 4217 (3-char)
//	location.language      hl               ISO 639-1 (2-char, from Accept-Language)
//	location.timezone      tz               IANA timezone (for time display)
type Response = domain.EnvironmentResponse
