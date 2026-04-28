package domain

import "context"

type LocationData struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	State       string  `json:"state"`
	Zipcode     string  `json:"zipcode"`
	Timezone    string  `json:"timezone"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

type WeatherData struct {
	Temp        float64 `json:"temp"`
	FeelsLike   float64 `json:"feels_like"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	IconURL     string  `json:"icon_url"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
}

type ContextResponse struct {
	Location LocationData `json:"location"`
	Weather  WeatherData  `json:"weather"`
}

type LocationProvider interface {
	ResolveIP(ctx context.Context, ip string) (*LocationData, error)
}

type WeatherProvider interface {
	GetCurrentWeather(ctx context.Context, lat, lon float64, lang string) (*WeatherData, error)
}
