package openweather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

const openWeatherBaseURL = "https://api.openweathermap.org/data/3.0/onecall"

type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient crea un cliente OpenWeather con timeout HTTP configurable.
// timeout es la duración máxima de cada petición HTTP al proveedor.
func NewClient(apiKey string, timeout time.Duration) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) GetCurrentWeather(ctx context.Context, lat, lon float64, lang, units string) (*domain.WeatherData, error) {
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
	q.Set("units", units)
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
		bodyStr := strings.TrimSpace(string(body))
		// HTTP 429 del proveedor externo → envolver con centinela ErrWeatherProviderRateLimited
		// para que el usecase pueda detectarlo con errors.Is y decidir entre propagar (429)
		// o degradar con gracia (weather null).
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("%w: openweather HTTP 429: %s", domain.ErrWeatherProviderRateLimited, bodyStr)
		}
		return nil, fmt.Errorf("openweather returned HTTP %d: %s", resp.StatusCode, bodyStr)
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

// =============================================================================
// Destination weather — forecast (≤7d) and historical (>7d)
// =============================================================================

// GetForecastForDate obtiene el pronóstico para una fecha específica usando la API onecall.
// Llama a /data/3.0/onecall con exclude=minutely,hourly,current,alerts y busca
// el daily[i] cuyo dt coincida con la fecha objetivo.
func (c *Client) GetForecastForDate(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
	if c.apiKey == "" {
		return nil, nil
	}

	targetDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("parse date %s: %w", date, err)
	}

	u, err := url.Parse(openWeatherBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse openweather URL: %w", err)
	}

	q := u.Query()
	q.Set("lat", fmt.Sprintf("%.6f", lat))
	q.Set("lon", fmt.Sprintf("%.6f", lng))
	q.Set("exclude", "minutely,hourly,current,alerts")
	q.Set("appid", c.apiKey)
	q.Set("units", "metric")
	q.Set("lang", "es")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("openweather create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openweather forecast request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		bodyStr := strings.TrimSpace(string(body))
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("%w: openweather forecast HTTP 429: %s", domain.ErrWeatherProviderRateLimited, bodyStr)
		}
		return nil, fmt.Errorf("openweather forecast returned HTTP %d: %s", resp.StatusCode, bodyStr)
	}

	var raw owDailyForecastResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("openweather forecast decode: %w", err)
	}

	// Buscar el daily entry que coincida con la fecha objetivo.
	// daily[0] es hoy, daily[1] mañana, etc.
	targetMidnight := targetDate.Unix()
	for _, d := range raw.Daily {
		dayStart := time.Unix(int64(d.Dt), 0).UTC().Truncate(24 * time.Hour).Unix()
		if dayStart == targetMidnight {
			return mapDailyToWeatherData(&d), nil
		}
	}

	return nil, fmt.Errorf("%w: no daily entry for %s", domain.ErrNoWeatherData, date)
}

// GetHistoricalForDate obtiene datos de clima histórico usando la API onecall/timemachine.
// Usa dt = fecha objetivo − 1 año al mediodía UTC, que es la práctica estándar de OpenWeather
// para estimar el clima probable en fechas lejanas.
func (c *Client) GetHistoricalForDate(ctx context.Context, lat, lng float64, date string) (*domain.WeatherData, error) {
	if c.apiKey == "" {
		return nil, nil
	}

	targetDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("parse date %s: %w", date, err)
	}

	// dt = target − 1 año, 12:00 UTC
	historicalDate := targetDate.AddDate(-1, 0, 0)
	historicalDT := time.Date(historicalDate.Year(), historicalDate.Month(), historicalDate.Day(), 12, 0, 0, 0, time.UTC)

	timemachineURL := "https://api.openweathermap.org/data/3.0/onecall/timemachine"

	u, err := url.Parse(timemachineURL)
	if err != nil {
		return nil, fmt.Errorf("parse timemachine URL: %w", err)
	}

	q := u.Query()
	q.Set("lat", fmt.Sprintf("%.6f", lat))
	q.Set("lon", fmt.Sprintf("%.6f", lng))
	q.Set("dt", fmt.Sprintf("%d", historicalDT.Unix()))
	q.Set("appid", c.apiKey)
	q.Set("units", "metric")
	q.Set("lang", "es")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("openweather timemachine create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openweather timemachine request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		bodyStr := strings.TrimSpace(string(body))
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("%w: openweather timemachine HTTP 429: %s", domain.ErrWeatherProviderRateLimited, bodyStr)
		}
		return nil, fmt.Errorf("openweather timemachine returned HTTP %d: %s", resp.StatusCode, bodyStr)
	}

	var raw owTimemachineResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("openweather timemachine decode: %w", err)
	}

	if len(raw.Data) == 0 {
		return nil, fmt.Errorf("%w: timemachine returned empty data for %s", domain.ErrNoWeatherData, date)
	}

	return mapTimemachineToWeatherData(&raw.Data[0]), nil
}

// =============================================================================
// Response structs for forecast and historical APIs
// =============================================================================

type owDailyForecastResponse struct {
	Daily []owDaily `json:"daily"`
}

type owDaily struct {
	Dt        int         `json:"dt"`
	Temp      owDailyTemp `json:"temp"`
	FeelsLike owFeels     `json:"feels_like"`
	Humidity  int         `json:"humidity"`
	WindSpeed float64     `json:"wind_speed"`
	Weather   []owWeather `json:"weather"`
}

type owDailyTemp struct {
	Day float64 `json:"day"`
}

type owFeels struct {
	Day float64 `json:"day"`
}

type owTimemachineResponse struct {
	Data []owTimemachineEntry `json:"data"`
}

type owTimemachineEntry struct {
	Temp      float64     `json:"temp"`
	FeelsLike float64     `json:"feels_like"`
	Humidity  int         `json:"humidity"`
	WindSpeed float64     `json:"wind_speed"`
	Weather   []owWeather `json:"weather"`
}

// =============================================================================
// Mapping helpers
// =============================================================================

func mapDailyToWeatherData(d *owDaily) *domain.WeatherData {
	icon := ""
	description := ""
	iconURL := ""

	if len(d.Weather) > 0 {
		icon = d.Weather[0].Icon
		description = d.Weather[0].Description
		iconURL = fmt.Sprintf("https://openweathermap.org/img/wn/%s@4x.png", icon)
	}

	return &domain.WeatherData{
		Temp:        d.Temp.Day,
		FeelsLike:   d.FeelsLike.Day,
		Description: description,
		Icon:        icon,
		IconURL:     iconURL,
		Humidity:    d.Humidity,
		WindSpeed:   d.WindSpeed,
	}
}

func mapTimemachineToWeatherData(e *owTimemachineEntry) *domain.WeatherData {
	icon := ""
	description := ""
	iconURL := ""

	if len(e.Weather) > 0 {
		icon = e.Weather[0].Icon
		description = e.Weather[0].Description
		iconURL = fmt.Sprintf("https://openweathermap.org/img/wn/%s@4x.png", icon)
	}

	return &domain.WeatherData{
		Temp:        e.Temp,
		FeelsLike:   e.FeelsLike,
		Description: description,
		Icon:        icon,
		IconURL:     iconURL,
		Humidity:    e.Humidity,
		WindSpeed:   e.WindSpeed,
	}
}

var _ domain.WeatherProvider = (*Client)(nil)
var _ domain.WeatherForecaster = (*Client)(nil)
