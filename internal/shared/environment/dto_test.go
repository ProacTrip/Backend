package environment_test

import (
	"encoding/json"
	"testing"

	sharedEnv "github.com/ProacTrip/Backend/internal/shared/environment"
)

func TestCacheEntry_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		entry   sharedEnv.CacheEntry
		wantKey string
	}{
		{
			name: "entrada completa con clima",
			entry: sharedEnv.CacheEntry{
				Location: sharedEnv.LocationDTO{
					Country:     "Argentina",
					CountryCode: "AR",
					City:        "Buenos Aires",
					Currency:    "ARS",
					Language:    "es",
					Latitude:    -34.6037,
					Longitude:   -58.3816,
				},
				Weather: &sharedEnv.WeatherDTO{
					Temp:        22.5,
					FeelsLike:   20.1,
					Description: "cielo claro",
					Icon:        "01d",
					IconURL:     "https://openweathermap.org/img/wn/01d@2x.png",
					Humidity:    55,
					WindSpeed:   3.6,
				},
			},
		},
		{
			name: "entrada sin clima (weather nil)",
			entry: sharedEnv.CacheEntry{
				Location: sharedEnv.LocationDTO{
					Country:     "United States",
					CountryCode: "US",
					City:        "New York",
					Currency:    "USD",
					Language:    "en",
					Latitude:    40.7128,
					Longitude:   -74.0060,
				},
				Weather: nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Serializar a JSON
			data, err := json.Marshal(tc.entry)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			// Deserializar
			var got sharedEnv.CacheEntry
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			// Verificar campos clave
			if got.Location.Country != tc.entry.Location.Country {
				t.Errorf("Country = %q, esperaba %q", got.Location.Country, tc.entry.Location.Country)
			}
			if got.Location.CountryCode != tc.entry.Location.CountryCode {
				t.Errorf("CountryCode = %q, esperaba %q", got.Location.CountryCode, tc.entry.Location.CountryCode)
			}

			// Weather nil debe seguir siendo nil después del round-trip
			if tc.entry.Weather == nil && got.Weather != nil {
				t.Error("Weather era nil pero después del round-trip no lo es")
			}
			if tc.entry.Weather != nil && got.Weather == nil {
				t.Error("Weather no era nil pero después del round-trip sí lo es")
			}
			if tc.entry.Weather != nil && got.Weather != nil {
				if got.Weather.Temp != tc.entry.Weather.Temp {
					t.Errorf("Weather.Temp = %f, esperaba %f", got.Weather.Temp, tc.entry.Weather.Temp)
				}
			}
		})
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{
			name: "IP pública",
			ip:   "8.8.8.8",
			want: "env:8.8.8.8",
		},
		{
			name: "IP localhost",
			ip:   "127.0.0.1",
			want: "env:127.0.0.1",
		},
		{
			name: "IP IPv6",
			ip:   "2001:4860:4860::8888",
			want: "env:2001:4860:4860::8888",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sharedEnv.CacheKey(tc.ip)
			if got != tc.want {
				t.Errorf("CacheKey(%q) = %q, esperaba %q", tc.ip, got, tc.want)
			}
		})
	}
}

func TestLocationDTO_AllFieldsSerialized(t *testing.T) {
	dto := sharedEnv.LocationDTO{
		Country:     "Spain",
		CountryCode: "ES",
		City:        "Madrid",
		State:       "Community of Madrid",
		Zipcode:     "28001",
		Timezone:    "Europe/Madrid",
		Currency:    "EUR",
		Language:    "es",
		Latitude:    40.4168,
		Longitude:   -3.7038,
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verificar que los 10 campos estén presentes
	requiredFields := []string{
		"country", "country_code", "city", "state", "zipcode",
		"timezone", "currency", "language", "latitude", "longitude",
	}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("campo %q ausente en JSON serializado", field)
		}
	}
}
