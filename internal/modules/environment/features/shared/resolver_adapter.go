// Package shared provee adaptadores transversales para el módulo environment.
//
// EnvironmentResolverAdapter tiende un puente entre las capacidades de geo-IP
// del módulo environment y la interfaz register.EnvironmentResolver del módulo auth.
// Implementa register.EnvironmentResolver de forma estructural — no necesita
// importar el módulo auth (el tipado estructural de Go lo resuelve).
package shared

import (
	"context"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
)

// =============================================================================
// EnvironmentResolverAdapter — adapta LocationProvider → register.EnvironmentResolver
// =============================================================================

// EnvironmentResolverAdapter resuelve moneda, idioma, código de país y
// timezone desde una IP usando el LocationProvider de geo-IP y el mapa
// estático CountryMetadata.
type EnvironmentResolverAdapter struct {
	locationProvider domain.LocationProvider
}

// NewEnvironmentResolverAdapter crea un adaptador respaldado por el LocationProvider dado.
func NewEnvironmentResolverAdapter(lp domain.LocationProvider) *EnvironmentResolverAdapter {
	return &EnvironmentResolverAdapter{locationProvider: lp}
}

// ResolveDefaults realiza una geo-búsqueda IP y enriquece el resultado con
// moneda e idioma desde el mapa CountryMetadata.
//
// Retorna (moneda, idioma, countryCode, timezone, error).
// Cuando el código de país no está en CountryMetadata, moneda e idioma se
// retornan como strings vacíos — el llamador (caso de uso de registro) los
// trata como "no resuelto" y continúa sin defaults de entorno.
func (a *EnvironmentResolverAdapter) ResolveDefaults(ctx context.Context, ip string) (currency, language, countryCode, timezone string, err error) {
	loc, err := a.locationProvider.ResolveIP(ctx, ip)
	if err != nil {
		return "", "", "", "", err
	}

	countryCode = loc.CountryCode
	timezone = loc.Timezone

	if info, ok := domain.CountryMetadata[countryCode]; ok {
		currency = info.Currency
		language = info.Language
	}
	// Si el código de país no está en CountryMetadata, moneda e idioma quedan ""
	// lo cual indica "no resuelto" al caso de uso de auth.

	return currency, language, countryCode, timezone, nil
}
