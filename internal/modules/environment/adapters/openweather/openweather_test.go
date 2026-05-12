package openweather

import (
	"testing"
)

// TestMapToWeatherData verifica el mapeo del response JSON de OpenWeather
// a domain.WeatherData en todos los escenarios: datos completos, weather vacío,
// y weather con múltiples entradas.
func TestMapToWeatherData(t *testing.T) {
	tests := []struct {
		name      string
		raw       *owOneCallResponse
		lang      string
		wantTemp  float64
		wantFeels float64
		wantHum   int
		wantWind  float64
		wantDesc  string
		wantIcon  string
		wantURL   string
	}{
		{
			name: "datos completos con weather — en",
			raw: &owOneCallResponse{
				Current: owCurrent{
					Temp:      22.5,
					FeelsLike: 20.1,
					Humidity:  65,
					WindSpeed: 3.6,
					Weather: []owWeather{
						{Description: "clear sky", Icon: "01d"},
					},
				},
			},
			lang:      "en",
			wantTemp:  22.5,
			wantFeels: 20.1,
			wantHum:   65,
			wantWind:  3.6,
			wantDesc:  "clear sky",
			wantIcon:  "01d",
			wantURL:   "https://openweathermap.org/img/wn/01d@4x.png",
		},
		{
			name: "weather vacío (nil) — es",
			raw: &owOneCallResponse{
				Current: owCurrent{
					Temp:      15.0,
					FeelsLike: 12.0,
					Humidity:  80,
					WindSpeed: 1.5,
					Weather:   nil,
				},
			},
			lang:      "es",
			wantTemp:  15.0,
			wantFeels: 12.0,
			wantHum:   80,
			wantWind:  1.5,
			wantDesc:  "",
			wantIcon:  "",
			wantURL:   "",
		},
		{
			name: "weather slice vacío (no nil) — fr",
			raw: &owOneCallResponse{
				Current: owCurrent{
					Temp:      10.0,
					FeelsLike: 8.5,
					Humidity:  90,
					WindSpeed: 2.0,
					Weather:   []owWeather{},
				},
			},
			lang:      "fr",
			wantTemp:  10.0,
			wantFeels: 8.5,
			wantHum:   90,
			wantWind:  2.0,
			wantDesc:  "",
			wantIcon:  "",
			wantURL:   "",
		},
		{
			name: "múltiples weather entries — toma el primero",
			raw: &owOneCallResponse{
				Current: owCurrent{
					Temp:      30.0,
					FeelsLike: 32.0,
					Humidity:  70,
					WindSpeed: 1.0,
					Weather: []owWeather{
						{Description: "tormenta", Icon: "11d"},
						{Description: "lluvia", Icon: "10d"},
					},
				},
			},
			lang:      "en",
			wantTemp:  30.0,
			wantFeels: 32.0,
			wantHum:   70,
			wantWind:  1.0,
			wantDesc:  "tormenta",
			wantIcon:  "11d",
			wantURL:   "https://openweathermap.org/img/wn/11d@4x.png",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wd := mapToWeatherData(tc.raw, tc.lang)

			if wd.Temp != tc.wantTemp {
				t.Errorf("temp = %f, esperaba %f", wd.Temp, tc.wantTemp)
			}
			if wd.FeelsLike != tc.wantFeels {
				t.Errorf("feels_like = %f, esperaba %f", wd.FeelsLike, tc.wantFeels)
			}
			if wd.Humidity != tc.wantHum {
				t.Errorf("humidity = %d, esperaba %d", wd.Humidity, tc.wantHum)
			}
			if wd.WindSpeed != tc.wantWind {
				t.Errorf("wind_speed = %f, esperaba %f", wd.WindSpeed, tc.wantWind)
			}
			if wd.Description != tc.wantDesc {
				t.Errorf("description = %q, esperaba %q", wd.Description, tc.wantDesc)
			}
			if wd.Icon != tc.wantIcon {
				t.Errorf("icon = %q, esperaba %q", wd.Icon, tc.wantIcon)
			}
			if wd.IconURL != tc.wantURL {
				t.Errorf("icon_url = %q, esperaba %q", wd.IconURL, tc.wantURL)
			}
		})
	}
}
