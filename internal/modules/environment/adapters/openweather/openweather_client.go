package openweather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

const openWeatherBaseURL = "https://api.openweathermap.org/data/3.0/onecall"

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) GetCurrentWeather(ctx context.Context, lat, lon float64, lang string) (*domain.WeatherData, error) {
	if c.apiKey == "" {
		return nil, nil
	}

	u, err := url.Parse(openWeatherBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse openweather URL: %w", err)
	}

	q := u.Query()
	q.Set("lat", fmt.Sprintf("%.6f", lat))
	q.Set("lon", fmt.Sprintf("%.6f", lon))
	q.Set("exclude", "minutely,hourly,daily,alerts")
	q.Set("appid", c.apiKey)
	q.Set("units", "metric")
	q.Set("lang", lang)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("openweather create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openweather request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("openweather returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw owOneCallResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("openweather decode response: %w", err)
	}

	return mapToWeatherData(&raw, lang), nil
}

type owOneCallResponse struct {
	Current owCurrent `json:"current"`
}

type owCurrent struct {
	Temp      float64    `json:"temp"`
	FeelsLike float64    `json:"feels_like"`
	Humidity  int        `json:"humidity"`
	WindSpeed float64    `json:"wind_speed"`
	Weather   []owWeather `json:"weather"`
}

type owWeather struct {
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

func mapToWeatherData(raw *owOneCallResponse, lang string) *domain.WeatherData {
	current := raw.Current

	icon := ""
	description := ""
	iconURL := ""

	if len(current.Weather) > 0 {
		icon = current.Weather[0].Icon
		description = current.Weather[0].Description
		iconURL = fmt.Sprintf("https://openweathermap.org/img/wn/%s@4x.png", icon)
	}

	return &domain.WeatherData{
		Temp:        current.Temp,
		FeelsLike:   current.FeelsLike,
		Description: description,
		Icon:        icon,
		IconURL:     iconURL,
		Humidity:    current.Humidity,
		WindSpeed:   current.WindSpeed,
	}
}

var _ domain.WeatherProvider = (*Client)(nil)
