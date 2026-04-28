package ipquery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/context/domain"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) ResolveIP(ctx context.Context, ip string) (*domain.LocationData, error) {
	url := fmt.Sprintf("%s/%s", c.baseURL, ip)

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

type ipQueryResponse struct {
	IP       string  `json:"ip"`
	ISP      string  `json:"isp"`
	Location ipQLocation `json:"location"`
	Risk     ipQRisk     `json:"risk"`
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
}

type ipQRisk struct {
	IsMobile    bool `json:"is_mobile"`
	IsVPN       bool `json:"is_vpn"`
	IsTor       bool `json:"is_tor"`
	IsProxy     bool `json:"is_proxy"`
	IsDatacenter bool `json:"is_datacenter"`
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
