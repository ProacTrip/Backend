// Tipos y puertos para la resolución de ubicación por IP.
package domain

import "context"

// LocationData contiene la información de ubicación geográfica de una IP.
type LocationData struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	State       string  `json:"state"`
	Zipcode     string  `json:"zipcode"`
	Timezone    string  `json:"timezone"`
	Currency    string  `json:"currency"`
	Language    string  `json:"language"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// LocationProvider es el puerto para resolver la ubicación geográfica de una IP.
type LocationProvider interface {
	ResolveIP(ctx context.Context, ip string) (*LocationData, error)
}
