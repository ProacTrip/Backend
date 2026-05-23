// Errores de dominio para el módulo environment.
// Mapeados a HTTP via RegisterDomainErrorMapper() en module.go.
package domain

import "errors"

// Errores de Validación y Disponibilidad
var (
	// ErrInvalidIP se retorna cuando la dirección IP del cliente es inválida,
	// malformada, privada o de loopback (HTTP 400).
	ErrInvalidIP = errors.New("dirección IP inválida")

	// ErrLocationProvider se retorna cuando el proveedor de geolocalización
	// (ipquery.io) no está disponible después de agotar los reintentos (HTTP 502).
	ErrLocationProvider = errors.New("proveedor de ubicación no disponible")

	// ErrRateLimitExceeded se retorna cuando se excede el límite de peticiones
	// configurado para el proveedor de clima (HTTP 429).
	ErrRateLimitExceeded = errors.New("límite de peticiones excedido")

	// ErrInternal se retorna para errores internos inesperados del servidor.
	ErrInternal = errors.New("error interno del servidor")

	// ErrWeatherProviderRateLimited es un centinela que envuelve el adaptador
	// OpenWeather cuando el proveedor externo responde HTTP 429.
	// Se detecta con errors.Is para decidir entre degradación elegante y propagación del error.
	ErrWeatherProviderRateLimited = errors.New("proveedor de clima: rate limit excedido")

	// ErrNoWeatherData se retorna cuando el provider de clima no encuentra datos
	// para la fecha solicitada (ej: forecast sin entrada para el día objetivo).
	// El usecase lo convierte en weather: null (degradación elegante).
	ErrNoWeatherData = errors.New("datos meteorológicos no disponibles para la fecha solicitada")

	// ErrInvalidParameterRange se retorna cuando un parámetro está fuera del rango válido.
	// Usado por get_destination_weather.Command.Validate() para lat/lng/date.
	ErrInvalidParameterRange = errors.New("parámetro fuera de rango")
)
