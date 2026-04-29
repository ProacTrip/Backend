package domain

import "context"

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

var countryCurrency = map[string]string{
	"ES": "EUR", "FR": "EUR", "DE": "EUR", "IT": "EUR", "PT": "EUR",
	"NL": "EUR", "BE": "EUR", "AT": "EUR", "FI": "EUR", "IE": "EUR",
	"GR": "EUR", "LU": "EUR", "MT": "EUR", "CY": "EUR", "SK": "EUR",
	"SI": "EUR", "EE": "EUR", "LV": "EUR", "LT": "EUR",
	"US": "USD", "EC": "USD", "SV": "USD", "PA": "USD",
	"GB": "GBP",
	"CH": "CHF",
	"JP": "JPY",
	"AU": "AUD",
	"CA": "CAD",
	"NZ": "NZD",
	"MX": "MXN",
	"BR": "BRL",
	"AR": "ARS",
	"CL": "CLP",
	"CO": "COP",
	"PE": "PEN",
	"DO": "DOP",
	"CR": "CRC",
	"IN": "INR",
	"CN": "CNY",
	"KR": "KRW",
	"RU": "RUB",
	"ZA": "ZAR",
	"TR": "TRY",
	"SE": "SEK",
	"NO": "NOK",
	"DK": "DKK",
	"PL": "PLN",
	"CZ": "CZK",
	"HU": "HUF",
	"TH": "THB",
	"SG": "SGD",
	"HK": "HKD",
	"MY": "MYR",
	"PH": "PHP",
	"ID": "IDR",
	"VN": "VND",
	"AE": "AED",
	"SA": "SAR",
	"IL": "ILS",
	"EG": "EGP",
	"NG": "NGN",
	"KE": "KES",
	"MA": "MAD",
	"UA": "UAH",
	"RO": "RON",
	"BG": "BGN",
	"IS": "ISK",
	"HR": "EUR",
}

func CurrencyForCountry(countryCode string) string {
	if currency, ok := countryCurrency[countryCode]; ok {
		return currency
	}
	return "USD"
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
