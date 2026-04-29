package get_context

import "github.com/ProacTrip/Backend/internal/modules/context/domain"

// Response = domain.ContextResponse
//
// SerpAPI Compatibility Mapping (for frontend developers):
//
//	Context Field        SerpAPI Parameter   Format
//	location.country_code  gl               ISO 3166-1 alpha-2 (2-char)
//	location.currency      currency         ISO 4217 (3-char)
//	location.language      hl               ISO 639-1 (2-char, from Accept-Language)
//	location.timezone      tz               IANA timezone (for time display)
type Response = domain.ContextResponse
