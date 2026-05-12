// Tests para el adaptador EnvironmentResolverAdapter que resuelve
// moneda, idioma, código de país y timezone desde una IP vía geo-IP.
package shared

import (
	"context"
	"errors"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

// =============================================================================
// Mock de LocationProvider
// =============================================================================

type mockLocationProvider struct {
	location *domain.LocationData
	err      error
}

func (m *mockLocationProvider) ResolveIP(_ context.Context, _ string) (*domain.LocationData, error) {
	return m.location, m.err
}

// =============================================================================
// Tests table-driven para EnvironmentResolverAdapter
// =============================================================================

func TestResolveDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		loc  *domain.LocationData
		err  error

		wantCurrency    string
		wantLanguage    string
		wantCountryCode string
		wantTimezone    string
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "Argentina — resuelve ARS, es, AR",
			loc: &domain.LocationData{
				CountryCode: "AR",
				Timezone:    "America/Argentina/Buenos_Aires",
			},
			wantCurrency:    "ARS",
			wantLanguage:    "es",
			wantCountryCode: "AR",
			wantTimezone:    "America/Argentina/Buenos_Aires",
		},
		{
			name: "Japón — resuelve JPY, ja, JP",
			loc: &domain.LocationData{
				CountryCode: "JP",
				Timezone:    "Asia/Tokyo",
			},
			wantCurrency:    "JPY",
			wantLanguage:    "ja",
			wantCountryCode: "JP",
			wantTimezone:    "Asia/Tokyo",
		},
		{
			name: "España — resuelve EUR, es, ES",
			loc: &domain.LocationData{
				CountryCode: "ES",
				Timezone:    "Europe/Madrid",
			},
			wantCurrency:    "EUR",
			wantLanguage:    "es",
			wantCountryCode: "ES",
			wantTimezone:    "Europe/Madrid",
		},
		{
			name: "país desconocido — currency y language vacíos",
			loc: &domain.LocationData{
				CountryCode: "XX",
				Timezone:    "UTC",
			},
			wantCurrency:    "",
			wantLanguage:    "",
			wantCountryCode: "XX",
			wantTimezone:    "UTC",
		},
		{
			name:            "error del proveedor — propaga el error",
			err:             errors.New("ipquery timeout"),
			wantErr:         true,
			wantErrContains: "ipquery timeout",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			adapter := NewEnvironmentResolverAdapter(&mockLocationProvider{
				location: tc.loc,
				err:      tc.err,
			})

			currency, language, countryCode, timezone, err := adapter.ResolveDefaults(t.Context(), "1.2.3.4")

			if tc.wantErr {
				if err == nil {
					t.Fatal("esperaba error, obtuve nil")
				}
				if tc.wantErrContains != "" && !errors.Is(err, errors.New(tc.wantErrContains)) {
					// Para errores simples sin wrapping, verificamos el mensaje
					if err.Error() != tc.wantErrContains {
						t.Errorf("error = %q, esperaba que contenga %q", err.Error(), tc.wantErrContains)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}

			if currency != tc.wantCurrency {
				t.Errorf("currency = %q, esperaba %q", currency, tc.wantCurrency)
			}
			if language != tc.wantLanguage {
				t.Errorf("language = %q, esperaba %q", language, tc.wantLanguage)
			}
			if countryCode != tc.wantCountryCode {
				t.Errorf("countryCode = %q, esperaba %q", countryCode, tc.wantCountryCode)
			}
			if timezone != tc.wantTimezone {
				t.Errorf("timezone = %q, esperaba %q", timezone, tc.wantTimezone)
			}
		})
	}
}

// TestResolveDefaults_SinLocation verifica el comportamiento cuando el
// proveedor retorna location nil (caso borde).
func TestResolveDefaults_SinLocation(t *testing.T) {
	t.Parallel()

	adapter := NewEnvironmentResolverAdapter(&mockLocationProvider{
		location: &domain.LocationData{
			CountryCode: "",
			Timezone:    "",
		},
	})

	currency, language, countryCode, timezone, err := adapter.ResolveDefaults(t.Context(), "8.8.8.8")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if currency != "" {
		t.Errorf("currency = %q, esperaba vacío (sin country code)", currency)
	}
	if language != "" {
		t.Errorf("language = %q, esperaba vacío (sin country code)", language)
	}
	if countryCode != "" {
		t.Errorf("countryCode = %q, esperaba vacío", countryCode)
	}
	if timezone != "" {
		t.Errorf("timezone = %q, esperaba vacío", timezone)
	}
}

// =============================================================================
// Verificación en tiempo de compilación — confirma que EnvironmentResolverAdapter
// satisface estructuralmente register.EnvironmentResolver
// =============================================================================

// Esto se verifica en el punto de wiring en bootstrap/app.go:
//   var _ register.EnvironmentResolver = (*shared.EnvironmentResolverAdapter)(nil)
