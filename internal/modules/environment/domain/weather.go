// Tipos y puertos para la obtención de datos meteorológicos.
package domain

import "context"

// WeatherData contiene la información meteorológica actual para una ubicación.
type WeatherData struct {
	Temp        float64 `json:"temp"`
	FeelsLike   float64 `json:"feels_like"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	IconURL     string  `json:"icon_url"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
}

// WeatherProvider es el puerto para obtener el clima actual de una ubicación.
// units determina la unidad de temperatura: "metric" (Celsius) o "imperial" (Fahrenheit).
type WeatherProvider interface {
	GetCurrentWeather(ctx context.Context, lat, lon float64, lang, units string) (*WeatherData, error)
}

// WeatherForecaster es el puerto para obtener el clima de un destino en una fecha específica.
// Soporta dos estrategias: forecast (≤7 días) y historical (>7 días, −1 año).
type WeatherForecaster interface {
	GetForecastForDate(ctx context.Context, lat, lng float64, date string) (*WeatherData, error)
	GetHistoricalForDate(ctx context.Context, lat, lng float64, date string) (*WeatherData, error)
}
