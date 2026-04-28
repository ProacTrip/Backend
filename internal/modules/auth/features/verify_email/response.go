package verify_email

// DTOs de respuesta según AUTH_API.md.

// Response es la respuesta del endpoint verify-email
type Response struct {
	User         UserResponse    `json:"user"`
	Context      ContextResponse `json:"context"`
	AccessToken  string          `json:"-"` // Para Set-Cookie, no en JSON
	RefreshToken string          `json:"-"` // Para Set-Cookie, no en JSON
}

// UserResponse contiene los datos del usuario verificado
type UserResponse struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	RoleName      string `json:"role_name"`
}

// ContextResponse contiene datos de contexto (location y weather)
type ContextResponse struct {
	Location LocationResponse `json:"location"`
	Weather  WeatherResponse  `json:"weather"`
}

// LocationResponse datos de ubicación según AUTH_API.md
type LocationResponse struct {
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	City        string `json:"city"`
	State       string `json:"state"`
	Timezone    string `json:"timezone"`
	Currency    string `json:"currency"`
	Language    string `json:"language"`
	Latitude    string `json:"latitude"`
	Longitude   string `json:"longitude"`
}

// WeatherResponse datos del clima según AUTH_API.md
type WeatherResponse struct {
	Temp        float64 `json:"temp"`
	FeelsLike   float64 `json:"feels_like"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	IconURL     string  `json:"icon_url"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
}
