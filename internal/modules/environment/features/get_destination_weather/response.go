// Response del caso de uso get_destination_weather.
// Reutiliza WeatherData del dominio environment para consistencia con el frontend.
package get_destination_weather

import "github.com/ProacTrip/Backend/internal/modules/environment/domain"

// Response es un alias del tipo WeatherData del dominio.
// Esto asegura que el frontend reciba exactamente el mismo formato
// que /v1/environment (temp, feels_like, description, icon, icon_url, humidity, wind_speed).
type Response = domain.WeatherData
