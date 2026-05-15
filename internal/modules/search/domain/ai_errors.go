// Referencia de errores de AI Search
//
// Este archivo documenta todos los tipos de error usados en el flujo ai_search.
// Errores de dominio (definidos en errors.go):
//   - ErrAIUnavailable → 503 Service Unavailable (proveedor AI caído)
//   - ErrAIParseFailure → 502 Bad Gateway (AI devolvió JSON inválido)
//   - ErrConversationNotFound → 400 Bad Request
//   - ErrTurnLimitExceeded → 400 Bad Request
//   - ErrProviderUnavailable → 502 Bad Gateway (todos los proveedores fallaron)
//   - ErrNoResults → manejado como respuesta vacía, no como error
//
// Errores de proveedor (desde features de búsqueda):
//   - ErrProviderUnavailable → 503 (SerpAPI 5xx o caído, definido en errors.go)
//   - ErrProviderBadRequest → 502 (SerpAPI 4xx, parámetros inválidos nuestros)
//   - ErrRateLimitExceeded → 429 (definido en shared/ratelimit)
//
// Errores de transporte (desde adapters):
//   - Errores de red → envueltos como ErrProviderUnavailable por el client
//   - Errores de decodificación JSON (SerpAPI 200 pero body inválido) → envueltos como ErrProviderUnavailable
//   - Mensajes de error de SerpAPI en el body de respuesta → envueltos como ErrProviderUnavailable
package domain
