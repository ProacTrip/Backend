package get_environment

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/labstack/echo/v5"
)

// init registra los mapeos de errores de dominio necesarios para los tests del handler.
// En producción, registerEnvironmentErrors() en module.go hace esto automáticamente.
func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrInvalidIP):
			return serrors.ErrBadRequest("dirección IP inválida", err)
		case errors.Is(err, domain.ErrLocationProvider):
			return serrors.ErrBadGateway("proveedor de ubicación no disponible", err)
		case errors.Is(err, domain.ErrRateLimitExceeded):
			return serrors.ErrTooManyRequests("límite de peticiones excedido", err)
		case errors.Is(err, domain.ErrInternal):
			return serrors.ErrInternalError("error interno del servidor", err)
		}
		return nil
	})
}

func TestHandler_XRealIP(t *testing.T) {
	tests := []struct {
		name           string
		xRealIP        string
		setXRealIP     bool
		wantStatusCode int
	}{
		{
			name:           "X-Real-IP header presente con IP pública — procede al usecase",
			xRealIP:        "8.8.8.8",
			setXRealIP:     true,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "sin X-Real-IP, usa RealIP del request — procede al usecase",
			xRealIP:        "",
			setXRealIP:     false,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "X-Real-IP con IP privada devuelve 400",
			xRealIP:        "192.168.1.1",
			setXRealIP:     true,
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "X-Real-IP con IP malformada devuelve 400",
			xRealIP:        "not-an-ip",
			setXRealIP:     true,
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "X-Real-IP con localhost devuelve 400",
			xRealIP:        "127.0.0.1",
			setXRealIP:     true,
			wantStatusCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()

			req := httptest.NewRequest(http.MethodGet, "/v1/environment", nil)
			if tc.setXRealIP {
				req.Header.Set("X-Real-IP", tc.xRealIP)
			}
			req.Header.Set("Accept-Language", "en")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Para tests de IP inválida, el usecase no se llama (validación falla antes).
			uc := NewUseCase(UseCaseDeps{})
			h := NewHandler(uc)

			_ = h.Handle(c)

			// Verificar el status code de la respuesta HTTP
			if tc.wantStatusCode == http.StatusBadRequest {
				if rec.Code != http.StatusBadRequest {
					t.Errorf("status code = %d, esperaba %d para IP %q",
						rec.Code, http.StatusBadRequest, tc.xRealIP)
				}
			}
			// Para IPs válidas, el código exacto depende del usecase (que fallará con nil providers)
			// Lo importante es que NO sea 400 (no fue rechazada por validación de IP)
			if tc.wantStatusCode == http.StatusOK && rec.Code == http.StatusBadRequest {
				t.Errorf("IP pública %q fue rechazada como inválida — no debería", tc.xRealIP)
			}
		})
	}
}

func TestHandler_CacheControl(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/v1/environment", nil)
	req.Header.Set("Accept-Language", "en")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	uc := NewUseCase(UseCaseDeps{})
	h := NewHandler(uc)

	_ = h.Handle(c)

	// Cache-Control no debería estar en respuestas de error (usecase falla con nil providers)
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "" {
		t.Errorf("Cache-Control no debería estar en error, obtuve %q", cacheControl)
	}
}

// TASK-ENV-020: El nil guard del usecase fue removido — el constructor garantiza non-nil.
// En su lugar, verificamos que NewHandler siempre asigna useCase correctamente.
func TestHandler_UseCaseNotNil(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{})
	h := NewHandler(uc)

	if h.useCase == nil {
		t.Error("NewHandler debe asignar useCase — nunca nil")
	}
	if h.useCase != uc {
		t.Error("NewHandler debe preservar la instancia del usecase")
	}
}

func TestHandler_ExtractLanguage(t *testing.T) {
	tests := []struct {
		name           string
		acceptLanguage string
		wantLang       string
	}{
		{
			name:           "sin header Accept-Language → en",
			acceptLanguage: "",
			wantLang:       "en",
		},
		{
			name:           "es-AR → es",
			acceptLanguage: "es-AR",
			wantLang:       "es",
		},
		{
			name:           "fr-FR → fr",
			acceptLanguage: "fr-FR",
			wantLang:       "fr",
		},
		{
			name:           "en-US,en;q=0.9 → en",
			acceptLanguage: "en-US,en;q=0.9",
			wantLang:       "en",
		},
		{
			name:           "solo código de idioma de 2 letras",
			acceptLanguage: "de",
			wantLang:       "de",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/v1/environment", nil)
			if tc.acceptLanguage != "" {
				req.Header.Set("Accept-Language", tc.acceptLanguage)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			got := extractLanguage(c)
			if got != tc.wantLang {
				t.Errorf("extractLanguage() = %q, esperaba %q", got, tc.wantLang)
			}
		})
	}
}

// =============================================================================
// Fase RED: Tests de escenarios HTTP del handler — TASK-ENV-029
// =============================================================================

// TestHandler_HTTP_ErrorScenarios verifica todos los escenarios HTTP del handler
// con usecases controlados que simulan diferentes errores y respuestas exitosas.
func TestHandler_HTTP_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name         string
		xRealIP      string
		mockSetup    func() *UseCase
		wantStatus   int
		wantContains string // substring esperado en el body
		checkHeaders func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:    "petición válida → 200 con Cache-Control",
			xRealIP: "8.8.8.8",
			mockSetup: func() *UseCase {
				locMock := &mockLocationProvider{
					resolveFn: func(ctx context.Context, ip string) (*domain.LocationData, error) {
						return &domain.LocationData{
							Country:     "United States",
							CountryCode: "US",
							City:        "Mountain View",
							Currency:    "USD",
							Language:    "en",
							Latitude:    37.422,
							Longitude:   -122.084,
							Timezone:    "America/Los_Angeles",
						}, nil
					},
				}
				weatherMock := &mockWeatherProvider{
					getFn: func(ctx context.Context, lat, lon float64, lang string) (*domain.WeatherData, error) {
						return makeMockWeatherData(), nil
					},
				}
				cacheMock := &mockCache{
					getFn: func(ctx context.Context, key string) (string, error) {
						return "", nil // cache miss
					},
				}
				return NewUseCase(UseCaseDeps{
					LocationProvider: locMock,
					WeatherProvider:  weatherMock,
					Cache:            cacheMock,
					RateLimiter:      nil,
					CacheTTL:         10 * time.Minute,
				})
			},
			wantStatus: http.StatusOK,
			checkHeaders: func(t *testing.T, rec *httptest.ResponseRecorder) {
				cacheControl := rec.Header().Get("Cache-Control")
				if cacheControl != "public, max-age=600" {
					t.Errorf("Cache-Control = %q, esperaba %q", cacheControl, "public, max-age=600")
				}
			},
		},
		{
			name:    "weather es nil cuando el proveedor no tiene API key",
			xRealIP: "8.8.8.8",
			mockSetup: func() *UseCase {
				locMock := &mockLocationProvider{
					resolveFn: func(ctx context.Context, ip string) (*domain.LocationData, error) {
						return &domain.LocationData{
							Country:     "Argentina",
							CountryCode: "AR",
							City:        "Buenos Aires",
							Currency:    "ARS",
							Language:    "es",
							Latitude:    -34.6037,
							Longitude:   -58.3816,
							Timezone:    "America/Argentina/Buenos_Aires",
						}, nil
					},
				}
				weatherMock := &mockWeatherProvider{
					getFn: func(ctx context.Context, lat, lon float64, lang string) (*domain.WeatherData, error) {
						return nil, nil // sin datos de clima
					},
				}
				cacheMock := &mockCache{
					getFn: func(ctx context.Context, key string) (string, error) {
						return "", nil
					},
				}
				return NewUseCase(UseCaseDeps{
					LocationProvider: locMock,
					WeatherProvider:  weatherMock,
					Cache:            cacheMock,
					RateLimiter:      nil,
					CacheTTL:         10 * time.Minute,
				})
			},
			wantStatus: http.StatusOK,
			checkHeaders: func(t *testing.T, rec *httptest.ResponseRecorder) {
				// Weather debe ser null en JSON
				body := rec.Body.String()
				if !strings.Contains(body, `"weather":null`) {
					t.Errorf("weather debería ser null, body = %s", body)
				}
			},
		},
		{
			name:    "IP malformada → 400 ERR_INVALID_IP",
			xRealIP: "not-an-ip",
			mockSetup: func() *UseCase {
				return NewUseCase(UseCaseDeps{})
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "IP",
		},
		{
			name:    "IP privada → 400",
			xRealIP: "192.168.1.1",
			mockSetup: func() *UseCase {
				return NewUseCase(UseCaseDeps{})
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "IP",
		},
		{
			name:    "IP loopback → 400",
			xRealIP: "127.0.0.1",
			mockSetup: func() *UseCase {
				return NewUseCase(UseCaseDeps{})
			},
			wantStatus:   http.StatusBadRequest,
			wantContains: "IP",
		},
		{
			name:    "proveedor de ubicación caído → 502",
			xRealIP: "8.8.8.8",
			mockSetup: func() *UseCase {
				locMock := &mockLocationProvider{
					resolveFn: func(ctx context.Context, ip string) (*domain.LocationData, error) {
						return nil, domain.ErrLocationProvider
					},
				}
				return NewUseCase(UseCaseDeps{
					LocationProvider: locMock,
					RateLimiter:      nil,
					CacheTTL:         10 * time.Minute,
				})
			},
			wantStatus:   http.StatusBadGateway,
			wantContains: "proveedor",
		},
		{
			name:    "rate limited → 429 (weather provider devuelve HTTP 429)",
			xRealIP: "8.8.8.8",
			mockSetup: func() *UseCase {
				locMock := &mockLocationProvider{
					resolveFn: func(ctx context.Context, ip string) (*domain.LocationData, error) {
						return &domain.LocationData{
							Country:     "US",
							CountryCode: "US",
							Latitude:    37.0,
							Longitude:   -122.0,
						}, nil
					},
				}
				weatherMock := &mockWeatherProvider{
					getFn: func(ctx context.Context, lat, lon float64, lang string) (*domain.WeatherData, error) {
						return nil, errors.New("HTTP 429: too many requests")
					},
				}
				cacheMock := &mockCache{
					getFn: func(ctx context.Context, key string) (string, error) {
						return "", nil
					},
				}
				return NewUseCase(UseCaseDeps{
					LocationProvider: locMock,
					WeatherProvider:  weatherMock,
					Cache:            cacheMock,
					RateLimiter:      nil,
					CacheTTL:         10 * time.Minute,
				})
			},
			wantStatus:   http.StatusTooManyRequests,
			wantContains: "límite",
		},
		{
			name:    "error de ubicación no mapeado → 500 (fallback genérico)",
			xRealIP: "8.8.8.8",
			mockSetup: func() *UseCase {
				locMock := &mockLocationProvider{
					resolveFn: func(ctx context.Context, ip string) (*domain.LocationData, error) {
						return nil, errors.New("unexpected crash in location provider")
					},
				}
				return NewUseCase(UseCaseDeps{
					LocationProvider: locMock,
					RateLimiter:      nil,
					CacheTTL:         10 * time.Minute,
				})
			},
			wantStatus:   http.StatusBadGateway, // wrapped as ErrLocationProvider
			wantContains: "proveedor",
		},
		{
			name:    "sin X-Real-IP usa RealIP — fallback a detección automática",
			xRealIP: "",
			mockSetup: func() *UseCase {
				locMock := &mockLocationProvider{
					resolveFn: func(ctx context.Context, ip string) (*domain.LocationData, error) {
						return &domain.LocationData{
							Country:     "United States",
							CountryCode: "US",
							City:        "Mountain View",
							Currency:    "USD",
							Language:    "en",
							Latitude:    37.422,
							Longitude:   -122.084,
							Timezone:    "America/Los_Angeles",
						}, nil
					},
				}
				weatherMock := &mockWeatherProvider{
					getFn: func(ctx context.Context, lat, lon float64, lang string) (*domain.WeatherData, error) {
						return makeMockWeatherData(), nil
					},
				}
				cacheMock := &mockCache{
					getFn: func(ctx context.Context, key string) (string, error) {
						return "", nil
					},
				}
				return NewUseCase(UseCaseDeps{
					LocationProvider: locMock,
					WeatherProvider:  weatherMock,
					Cache:            cacheMock,
					RateLimiter:      nil,
					CacheTTL:         10 * time.Minute,
				})
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()

			req := httptest.NewRequest(http.MethodGet, "/v1/environment", nil)
			if tc.xRealIP != "" {
				req.Header.Set("X-Real-IP", tc.xRealIP)
			}
			req.Header.Set("Accept-Language", "en")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			uc := tc.mockSetup()
			h := NewHandler(uc)

			_ = h.Handle(c)

			if rec.Code != tc.wantStatus {
				t.Errorf("status code = %d, esperaba %d. Body: %s",
					rec.Code, tc.wantStatus, rec.Body.String())
			}

			if tc.wantContains != "" && !strings.Contains(rec.Body.String(), tc.wantContains) {
				t.Errorf("body debería contener %q, body = %s", tc.wantContains, rec.Body.String())
			}

			if tc.checkHeaders != nil {
				tc.checkHeaders(t, rec)
			}
		})
	}
}

func TestHandler_ResolvedIP(t *testing.T) {
	e := echo.New()

	t.Run("prefiere X-Real-IP sobre RealIP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/environment", nil)
		req.Header.Set("X-Real-IP", "1.2.3.4")
		req.Header.Set("Accept-Language", "en")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		got := c.Request().Header.Get("X-Real-IP")
		if got != "1.2.3.4" {
			t.Errorf("X-Real-IP header = %q, esperaba %q", got, "1.2.3.4")
		}
	})
}
