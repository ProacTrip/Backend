// Cliente HTTP para SerpAPI.
// Comunica con la API externa de búsqueda de vuelos.
package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

const serpapiBaseURL = "https://serpapi.com/search"

var defaultHTTPTimeout = 30 * time.Second

// Client performs SerpAPI google_flights searches via raw HTTP with context support.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new SerpAPI HTTP client.
// If apiKey is empty, a warning is logged but the client is still created
// — requests will fail with a SerpAPI authentication error at call time.
func NewClient(apiKey string, timeout time.Duration) *Client {
	if apiKey == "" {
		slog.Error("SERPAPI_KEY is empty — serpapi requests will fail")
	}
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Search performs a flight search via the SerpAPI HTTP API.
func (c *Client) Search(ctx context.Context, params map[string]string) (map[string]interface{}, error) {
	return c.doRequest(ctx, params)
}

// GetBookingDetails retrieves booking details via the SerpAPI HTTP API.
func (c *Client) GetBookingDetails(ctx context.Context, bookingToken string, adults int, currency string, departureID, arrivalID, outboundDate, returnDate string) (map[string]interface{}, error) {
	params := map[string]string{
		"booking_token": bookingToken,
		"departure_id":  departureID,
		"arrival_id":    arrivalID,
		"outbound_date": outboundDate,
		"adults":        fmt.Sprintf("%d", adults),
		"currency":      currency,
	}
	if returnDate != "" {
		params["return_date"] = returnDate
	}
	return c.doRequest(ctx, params)
}

// SearchHotels performs a hotel search via the SerpAPI HTTP API using google_hotels engine.
func (c *Client) SearchHotels(ctx context.Context, params map[string]string) (map[string]interface{}, error) {
	return c.doRequestWithEngine(ctx, params, "google_hotels")
}

// GetHotelDetails retrieves hotel property details via the SerpAPI HTTP API.
func (c *Client) GetHotelDetails(ctx context.Context, params map[string]string) (map[string]interface{}, error) {
	return c.doRequestWithEngine(ctx, params, "google_hotels")
}

// doRequest builds and executes an HTTP GET request to SerpAPI.
// Respects context cancellation for graceful shutdown.
func (c *Client) doRequest(ctx context.Context, params map[string]string) (map[string]interface{}, error) {
	return c.doRequestWithEngine(ctx, params, "google_flights")
}

// doRequestWithEngine builds and executes an HTTP GET request to SerpAPI with a given engine.
func (c *Client) doRequestWithEngine(ctx context.Context, params map[string]string, engine string) (map[string]interface{}, error) {
	u, err := url.Parse(serpapiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse serpapi URL: %w", err)
	}

	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	q.Set("engine", engine)
	q.Set("api_key", c.apiKey)
	u.RawQuery = q.Encode()

	slog.InfoContext(ctx, "serpapi request",
		slog.String("engine", engine),
		slog.String("url", maskSensitiveURL(u.String())),
		slog.Bool("has_api_key", c.apiKey != ""),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create serpapi request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: serpapi request: %w", domain.ErrProviderUnavailable, err)
	}

	slog.InfoContext(ctx, "serpapi response",
		slog.String("engine", engine),
		slog.Int("status", resp.StatusCode),
	)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		errorBody := strings.TrimSpace(string(bodyBytes))

		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("%w: serpapi internal error (HTTP %d)", domain.ErrProviderUnavailable, resp.StatusCode)
		}
		// 4xx errors are our fault (bad params) — map to 502 Bad Gateway upstream
		if errorBody != "" {
			return nil, fmt.Errorf("%w: serpapi returned HTTP %d: %s", domain.ErrProviderBadRequest, resp.StatusCode, errorBody)
		}
		return nil, fmt.Errorf("%w: serpapi returned HTTP %d", domain.ErrProviderBadRequest, resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: decode serpapi response: %w", domain.ErrProviderUnavailable, err)
	}

	if errMsg := getErrorFromResponse(result); errMsg != "" {
		return nil, fmt.Errorf("%w: %s", domain.ErrProviderUnavailable, errMsg)
	}

	return result, nil
}

// maskSensitiveURL masks the api_key query parameter in a URL for safe logging.
func maskSensitiveURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Get("api_key") != "" {
		q.Set("api_key", "***")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// getErrorFromResponse extracts an error message from a SerpAPI response map.
func getErrorFromResponse(result map[string]interface{}) string {
	meta, ok := result["search_metadata"].(map[string]interface{})
	if !ok {
		return ""
	}

	status, _ := meta["status"].(string)
	if status == "Success" || status == "" {
		return ""
	}

	if errMsg, ok := result["error"].(string); ok && errMsg != "" {
		return errMsg
	}

	return status
}
