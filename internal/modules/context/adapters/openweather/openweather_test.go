package openweather

import (
	"testing"
)

func TestMapToWeatherData(t *testing.T) {
	raw := &owOneCallResponse{
		Current: owCurrent{
			Temp:      22.5,
			FeelsLike: 20.1,
			Humidity:  65,
			WindSpeed: 3.6,
			Weather: []owWeather{
				{Description: "clear sky", Icon: "01d"},
			},
		},
	}

	wd := mapToWeatherData(raw, "en")

	if wd.Temp != 22.5 {
		t.Errorf("expected temp 22.5, got %f", wd.Temp)
	}
	if wd.FeelsLike != 20.1 {
		t.Errorf("expected feels_like 20.1, got %f", wd.FeelsLike)
	}
	if wd.Description != "clear sky" {
		t.Errorf("expected description 'clear sky', got '%s'", wd.Description)
	}
	if wd.Icon != "01d" {
		t.Errorf("expected icon '01d', got '%s'", wd.Icon)
	}
	if wd.IconURL != "https://openweathermap.org/img/wn/01d@4x.png" {
		t.Errorf("expected icon_url, got '%s'", wd.IconURL)
	}
	if wd.Humidity != 65 {
		t.Errorf("expected humidity 65, got %d", wd.Humidity)
	}
	if wd.WindSpeed != 3.6 {
		t.Errorf("expected wind_speed 3.6, got %f", wd.WindSpeed)
	}
}

func TestMapToWeatherDataEmptyWeather(t *testing.T) {
	raw := &owOneCallResponse{
		Current: owCurrent{
			Temp:      15.0,
			FeelsLike: 12.0,
			Humidity:  80,
			WindSpeed: 1.5,
			Weather:   nil,
		},
	}

	wd := mapToWeatherData(raw, "es")

	if wd.Icon != "" {
		t.Errorf("expected empty icon, got '%s'", wd.Icon)
	}
	if wd.IconURL != "" {
		t.Errorf("expected empty icon_url, got '%s'", wd.IconURL)
	}
	if wd.Description != "" {
		t.Errorf("expected empty description, got '%s'", wd.Description)
	}
}
