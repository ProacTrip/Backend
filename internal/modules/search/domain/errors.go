// Definición de errores de dominio para el módulo de búsqueda.
// Errores específicos del negocio.
package domain

import "errors"

var (
	ErrInvalidTripType      = errors.New("INVALID_TRIP_TYPE: el tipo de viaje no es válido")
	ErrMissingRequiredField = errors.New("MISSING_REQUIRED_FIELD: falta un campo requerido")
	ErrInvalidParameterRange = errors.New("INVALID_PARAMETER_RANGE: parámetro fuera de rango")
	ErrProviderUnavailable  = errors.New("PROVIDER_UNAVAILABLE: el proveedor externo no está disponible")
	ErrProviderBadRequest   = errors.New("PROVIDER_BAD_REQUEST: el proveedor rechazó la solicitud por parámetros inválidos")
	ErrProviderError        = errors.New("PROVIDER_ERROR: error del proveedor externo")
	ErrNoResults            = errors.New("NO_RESULTS: no se encontraron resultados")
	ErrTokenInvalid         = errors.New("TOKEN_INVALID: el token es inválido o ha expirado")
	ErrTokenRequired        = errors.New("TOKEN_REQUIRED: se requiere un token para esta operación")
	ErrCacheFailed          = errors.New("CACHE_FAILED: error al acceder a la caché")
	ErrRateLimitExceeded    = errors.New("RATE_LIMIT_EXCEEDED: límite de solicitudes excedido")

	// AI Search errors
	ErrAIUnavailable         = errors.New("AI_UNAVAILABLE: el servicio de IA no está disponible")
	ErrAIParseFailure        = errors.New("AI_PARSE_FAILURE: la IA devolvió una respuesta inválida o malformada")
	ErrConversationNotFound  = errors.New("CONVERSATION_NOT_FOUND: conversation_id no encontrado")
	ErrTurnLimitExceeded     = errors.New("TURN_LIMIT_EXCEEDED: se alcanzó el límite máximo de turnos")

	// Search provider errors
	// ErrSearchFailed removido — sin referencias, usaba ErrProviderUnavailable en su lugar

	// Discovery errors
	ErrDiscoveryDisabled = errors.New("DISCOVERY_DISABLED: el modo discovery no está habilitado")
	ErrNoCandidatesFound = errors.New("NO_CANDIDATES: no se encontraron destinos que coincidan con los criterios")
	ErrClarifyMaxRounds = errors.New("CLARIFY_MAX_ROUNDS: se alcanzó el límite de preguntas de aclaración")

	// Booking & property errors
	ErrBookingTokenExpired = errors.New("BOOKING_TOKEN_EXPIRED: el token de reserva ha expirado o no es válido")
	ErrPropertyNotFound    = errors.New("PROPERTY_NOT_FOUND: la propiedad no fue encontrada")
)
