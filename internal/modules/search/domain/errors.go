// Definición de errores de dominio para el módulo de búsqueda.
// Errores específicos del negocio.
package domain

import "errors"

var (
	ErrInvalidTripType      = errors.New("INVALID_TRIP_TYPE: el tipo de viaje no es válido")
	ErrMissingRequiredField = errors.New("MISSING_REQUIRED_FIELD: falta un campo requerido")
	ErrInvalidParameterRange = errors.New("INVALID_PARAMETER_RANGE: parámetro fuera de rango")
	ErrProviderUnavailable  = errors.New("PROVIDER_UNAVAILABLE: el proveedor externo no está disponible")
	ErrProviderError        = errors.New("PROVIDER_ERROR: error del proveedor externo")
	ErrNoResults            = errors.New("NO_RESULTS: no se encontraron resultados")
	ErrTokenInvalid         = errors.New("TOKEN_INVALID: el token es inválido o ha expirado")
	ErrTokenRequired        = errors.New("TOKEN_REQUIRED: se requiere un token para esta operación")
	ErrCacheFailed          = errors.New("CACHE_FAILED: error al acceder a la caché")
)
