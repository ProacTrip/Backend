package ipquery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

// =============================================================================
// Helpers de construcción
// =============================================================================

func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func validIPQueryResponse() map[string]interface{} {
	return map[string]interface{}{
		"ip":  "8.8.8.8",
		"isp": map[string]interface{}{
			"asn": "AS15169",
			"org": "Google LLC",
			"isp": "Google LLC",
		},
		"location": map[string]interface{}{
			"country":      "United States",
			"country_code": "US",
			"city":         "Mountain View",
			"state":        "California",
			"zipcode":      "94043",
			"timezone":     "America/Los_Angeles",
			"latitude":     37.422,
			"longitude":    -122.084,
		},
		"risk": map[string]interface{}{
			"is_mobile":     false,
			"is_vpn":        false,
			"is_tor":        false,
			"is_proxy":      false,
			"is_datacenter": true,
		},
	}
}

// =============================================================================
// TASK-ENV-016: Retry with exponential backoff
// =============================================================================

func TestClient_ResolveIP_RetryOnTransientErrors(t *testing.T) {
	tests := []struct {
		name         string
		statusCodes  []int // sequence of HTTP status codes per attempt
		maxRetries   int
		wantErr      bool
		wantAttempts int // expected number of HTTP requests made
	}{
		{
			name:         "success on first attempt — no retry",
			statusCodes:  []int{http.StatusOK},
			maxRetries:   3,
			wantErr:      false,
			wantAttempts: 1,
		},
		{
			name:         "success on second attempt after 500",
			statusCodes:  []int{http.StatusInternalServerError, http.StatusOK},
			maxRetries:   3,
			wantErr:      false,
			wantAttempts: 2,
		},
		{
			name:         "all retries exhausted with 500",
			statusCodes:  []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError},
			maxRetries:   3,
			wantErr:      true,
			wantAttempts: 4, // initial + 3 retries
		},
		{
			name:         "no retry on 400 (client error)",
			statusCodes:  []int{http.StatusBadRequest},
			maxRetries:   3,
			wantErr:      true,
			wantAttempts: 1,
		},
		{
			name:         "no retry on 429 (rate limit)",
			statusCodes:  []int{http.StatusTooManyRequests},
			maxRetries:   3,
			wantErr:      true,
			wantAttempts: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attemptCount := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				idx := attemptCount
				if idx >= len(tc.statusCodes) {
					idx = len(tc.statusCodes) - 1
				}
				status := tc.statusCodes[idx]
				attemptCount++

				w.WriteHeader(status)
				if status == http.StatusOK {
					resp := validIPQueryResponse()
					json.NewEncoder(w).Encode(resp)
				}
			}))
			defer ts.Close()

			client := NewClient(ts.URL, 5*time.Second, tc.maxRetries)
			ctx := t.Context()

			_, err := client.ResolveIP(ctx, "8.8.8.8")

			if tc.wantErr && err == nil {
				t.Error("esperaba error, obtuve nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("error inesperado: %v", err)
			}
			if attemptCount != tc.wantAttempts {
				t.Errorf("attempts = %d, esperaba %d", attemptCount, tc.wantAttempts)
			}
		})
	}
}

func TestClient_ResolveIP_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, 100*time.Millisecond, 1)
	ctx := t.Context()

	_, err := client.ResolveIP(ctx, "8.8.8.8")
	if err == nil {
		t.Fatal("esperaba timeout error, obtuve nil")
	}
}

// =============================================================================
// TASK-ENV-017: ISP field type fix (ipQISP struct)
// =============================================================================

func TestClient_ResolveIP_ISPStructParsing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"ip": "1.2.3.4",
			"isp": map[string]interface{}{
				"asn": "AS12345",
				"org": "Test ISP Org",
				"isp": "Test ISP",
			},
			"location": map[string]interface{}{
				"country":      "Argentina",
				"country_code": "AR",
				"city":         "Buenos Aires",
				"state":        "Buenos Aires",
				"zipcode":      "1001",
				"timezone":     "America/Argentina/Buenos_Aires",
				"latitude":     -34.6037,
				"longitude":    -58.3816,
			},
			"risk": map[string]interface{}{
				"is_mobile":     false,
				"is_vpn":        false,
				"is_tor":        false,
				"is_proxy":      false,
				"is_datacenter": false,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, 5*time.Second, 1)
	ctx := t.Context()

	loc, err := client.ResolveIP(ctx, "1.2.3.4")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if loc.Country != "Argentina" {
		t.Errorf("country = %q, esperaba %q", loc.Country, "Argentina")
	}
	if loc.City != "Buenos Aires" {
		t.Errorf("city = %q, esperaba %q", loc.City, "Buenos Aires")
	}
	// Datos de ISP no deben estar en LocationData
}

func TestClient_ResolveIP_ISPFieldNull(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"ip":  "5.6.7.8",
			"isp": nil,
			"location": map[string]interface{}{
				"country":      "Japan",
				"country_code": "JP",
				"city":         "Tokyo",
				"state":        "Tokyo",
				"zipcode":      "100-0001",
				"timezone":     "Asia/Tokyo",
				"latitude":     35.6895,
				"longitude":    139.6917,
			},
			"risk": nil,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, 5*time.Second, 1)
	ctx := t.Context()

	loc, err := client.ResolveIP(ctx, "5.6.7.8")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if loc.Country != "Japan" {
		t.Errorf("country = %q, esperaba %q", loc.Country, "Japan")
	}
}

// =============================================================================
// Manejo de códigos de estado HTTP
// =============================================================================

func TestClient_ResolveIP_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "200 OK → success",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "400 Bad Request → error",
			statusCode: http.StatusBadRequest,
			wantErr:    true,
			errMsg:     "invalid IP format",
		},
		{
			name:       "429 Too Many Requests → error",
			statusCode: http.StatusTooManyRequests,
			wantErr:    true,
			errMsg:     "rate limited",
		},
		{
			name:       "500 → error after retries",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.statusCode == http.StatusOK {
					resp := validIPQueryResponse()
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
					return
				}
				w.WriteHeader(tc.statusCode)
			}))
			defer ts.Close()

			client := NewClient(ts.URL, 5*time.Second, 1)
			ctx := t.Context()

			_, err := client.ResolveIP(ctx, "8.8.8.8")

			if tc.wantErr && err == nil {
				t.Error("esperaba error, obtuve nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("error inesperado: %v", err)
			}
			if tc.errMsg != "" && err != nil && !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("error = %q, esperaba que contenga %q", err.Error(), tc.errMsg)
			}
		})
	}
}

// =============================================================================
// Manejo de errores de decodificación JSON
// =============================================================================

func TestClient_ResolveIP_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, 5*time.Second, 1)
	ctx := t.Context()

	_, err := client.ResolveIP(ctx, "8.8.8.8")
	if err == nil {
		t.Fatal("esperaba error de JSON decode, obtuve nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %q, esperaba error de decode", err.Error())
	}
}

// =============================================================================
// Verificación en tiempo de compilación
// =============================================================================

func TestClient_ImplementsLocationProvider(t *testing.T) {
	// Verificación en tiempo de compilación: var _ domain.LocationProvider = (*Client)(nil)
	// ya existe en ipquery_client.go — este test lo documenta.
	var _ domain.LocationProvider = (*Client)(nil)
	_ = t
}

// =============================================================================
// Cálculo de backoff exponencial
// =============================================================================

func TestRetryBackoff(t *testing.T) {
	// Verificar duración de backoff para cada intento de reintento
	// Intento 0 (inicial): 0s
	// Reintento 1: 1s, Reintento 2: 2s, Reintento 3: 4s
	tests := []struct {
		attempt  int
		wantMin  time.Duration
		wantMax  time.Duration
	}{
		{attempt: 0, wantMin: 0, wantMax: 0},
		{attempt: 1, wantMin: 1 * time.Second, wantMax: 1 * time.Second},
		{attempt: 2, wantMin: 2 * time.Second, wantMax: 2 * time.Second},
		{attempt: 3, wantMin: 4 * time.Second, wantMax: 4 * time.Second},
		{attempt: 4, wantMin: 8 * time.Second, wantMax: 8 * time.Second},
	}

	for _, tc := range tests {
		got := backoffDuration(tc.attempt)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("backoffDuration(%d) = %v, esperaba entre %v y %v",
				tc.attempt, got, tc.wantMin, tc.wantMax)
		}
	}
}
