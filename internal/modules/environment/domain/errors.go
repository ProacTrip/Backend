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
)
