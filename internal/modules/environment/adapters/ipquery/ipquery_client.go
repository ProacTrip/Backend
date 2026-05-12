package ipquery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// NewClient crea un cliente para la API de ipquery.io.
// timeout: timeout HTTP por intento individual.
// maxRetries: número máximo de reintentos (0 = sin reintentos).
func NewClient(baseURL string, timeout time.Duration, maxRetries int) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		maxRetries: maxRetries,
	}
}

// backoffDuration calcula el tiempo de espera exponencial para un intento dado.
// Intento 0 → 0s, intento 1 → 1s, intento 2 → 2s, intento 3 → 4s.
func backoffDuration(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}

func (c *Client) ResolveIP(ctx context.Context, ip string) (*domain.LocationData, error) {
	url := c.baseURL
	if ip != "" {
		url = fmt.Sprintf("%s/%s", c.baseURL, ip)
	}

	maxAttempts := c.maxRetries + 1 // initial attempt + retries

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := backoffDuration(attempt)
			slog.WarnContext(ctx, "ipquery retry",
				"attempt", attempt,
				"max_retries", c.maxRetries,
				"backoff", backoff,
				"ip", ip,
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, fmt.Errorf("ipquery retry cancelled: %w", ctx.Err())
			}
		}

		start := time.Now()
		result, err := c.resolveIPOnce(ctx, url)
		elapsed := time.Since(start)

		if err == nil {
			slog.DebugContext(ctx, "ipquery request successful",
				"duration_ms", elapsed.Milliseconds(),
				"ip", ip,
			)
			return result, nil
		}

		lastErr = err

		// No reintentar en errores de cliente (400, 429) o errores no transitorios
		if !isRetryable(err) {
			slog.DebugContext(ctx, "ipquery error no reintentable",
				"error", err,
				"duration_ms", elapsed.Milliseconds(),
				"ip", ip,
			)
			break
		}

		slog.WarnContext(ctx, "ipquery transient error, will retry",
			"error", err,
			"attempt", attempt,
			"duration_ms", elapsed.Milliseconds(),
			"ip", ip,
		)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("ipquery request failed after %d attempts: %w", maxAttempts, lastErr)
	}
	return nil, fmt.Errorf("ipquery: unexpected nil error after %d attempts", maxAttempts)
}

func (c *Client) resolveIPOnce(ctx context.Context, url string) (*domain.LocationData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ipquery create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ipquery request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest:
		return nil, fmt.Errorf("ipquery bad request: invalid IP format")
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("ipquery rate limited")
	case http.StatusInternalServerError:
		return nil, fmt.Errorf("ipquery internal server error")
	default:
		return nil, fmt.Errorf("ipquery returned HTTP %d", resp.StatusCode)
	}

	var raw ipQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("ipquery decode response: %w", err)
	}

	return mapToDomain(&raw), nil
}

// isRetryable determina si un error merece reintento.
// Solo reintentar en errores transitorios (timeout, 5xx).
// NO reintentar en 400 (client error), 429 (rate limit), o parse errors.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()

	// No reintentar errores de cliente
	if containsAny(errStr, "invalid IP format", "rate limited") {
		return false
	}
	// No reintentar errores de parse
	if containsAny(errStr, "decode response") {
		return false
	}
	// Reintentar timeouts, 5xx, y errores de red
	return true
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && len(s) >= len(sub) {
			// Búsqueda simple de substring
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

type ipQISP struct {
	ASN string `json:"asn"`
	Org string `json:"org"`
	ISP string `json:"isp"`
}

type ipQueryResponse struct {
	// ISP es capturado para uso futuro (analytics, logging).
	ISP      ipQISP      `json:"isp"`
	Location ipQLocation `json:"location"`
}

type ipQLocation struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	State       string  `json:"state"`
	Zipcode     string  `json:"zipcode"`
	Timezone    string  `json:"timezone"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	LocalTime   string  `json:"localtime"`
}

func mapToDomain(raw *ipQueryResponse) *domain.LocationData {
	return &domain.LocationData{
		Country:     raw.Location.Country,
		CountryCode: raw.Location.CountryCode,
		City:        raw.Location.City,
		State:       raw.Location.State,
		Zipcode:     raw.Location.Zipcode,
		Timezone:    raw.Location.Timezone,
		Latitude:    raw.Location.Latitude,
		Longitude:   raw.Location.Longitude,
	}
}

var _ domain.LocationProvider = (*Client)(nil)
