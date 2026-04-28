package login

// DTOs del login según AUTH_API.md.

type Command struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type Response struct {
	AccessToken  string        `json:"-"`
	RefreshToken string        `json:"-"`
	User         *UserResponse `json:"user"`
	Context      *ContextData  `json:"context,omitempty"`
}

type UserResponse struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	RoleName      string `json:"role_name"`
}

type ContextData struct {
	Location LocationData `json:"location"`
	Weather  WeatherData  `json:"weather"`
}

type LocationData struct {
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name,omitempty"`
	City        string `json:"city"`
	State       string `json:"state,omitempty"`
	Timezone    string `json:"timezone"`
	Currency    string `json:"currency"`
	Language    string `json:"language"`
	Latitude    string `json:"latitude,omitempty"`
	Longitude   string `json:"longitude,omitempty"`
}

type WeatherData struct {
	Temp      float64 `json:"temperature_c"`
	Condition string  `json:"condition"`
	Humidity  int     `json:"humidity_percent"`
}
