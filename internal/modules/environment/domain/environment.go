// Estructura principal de respuesta del módulo environment.
// Contiene tanto datos de ubicación como meteorológicos.
package domain

// EnvironmentResponse es la respuesta unificada de ubicación geográfica y clima.
// Weather es *WeatherData (nullable) — cuando no hay datos de clima,
// el campo se serializa como null, no se omite.
type EnvironmentResponse struct {
	Location LocationData `json:"location"`
	Weather  *WeatherData `json:"weather"`
}
