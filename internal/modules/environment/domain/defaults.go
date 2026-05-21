// Funciones de utilidad y valores por defecto para el módulo environment.
// Incluye DefaultLocation() para desarrollo local e IsPrivateOrLocalIP()
// para validación de IP en el handler.
package domain

import (
	"net"
)

const (
	DefaultCountry     = "Argentina"
	DefaultCountryCode = "AR"
	DefaultCity        = "Buenos Aires"
	DefaultCurrency    = "USD"
	DefaultLanguage    = "es"
	DefaultLatitude    = -34.6037
	DefaultLongitude   = -58.3816
)

// DefaultLocation retorna una ubicación por defecto para entornos de desarrollo
// o cuando no se puede resolver la IP del cliente.
// countryCode es el código ISO 3166-1 alpha-2 configurado (ej: "AR").
// Si no se encuentra en CountryMetadata, devuelve Buenos Aires, Argentina.
func DefaultLocation(countryCode string) *LocationData {
	if cc := countryCode; cc != "" {
		if info, ok := CountryMetadata[cc]; ok {
			return &LocationData{
				Country:     info.Country,
				CountryCode: cc,
				Currency:    info.Currency,
				Language:    info.Language,
			}
		}
	}
	return &LocationData{
		Country:     DefaultCountry,
		CountryCode: DefaultCountryCode,
		City:        DefaultCity,
		Currency:    DefaultCurrency,
		Language:    DefaultLanguage,
		Latitude:    DefaultLatitude,
		Longitude:   DefaultLongitude,
	}
}

// IsPrivateOrLocalIP determina si una dirección IP es privada, de loopback,
// no especificada o malformada. Se usa en el handler para rechazar IPs
// que no deberían llegar al proveedor de geolocalización (HTTP 400).
func IsPrivateOrLocalIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return true // malformada o vacía → tratar como privada
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsUnspecified()
}
