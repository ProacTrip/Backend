// Puerto para acceder a búsquedas guardadas desde el módulo user.
// El módulo search consume búsquedas guardadas sin acoplarse al módulo user.
//
// SavedSearchProvider y SavedSearchData están definidos en shared/search/contract.go
// para que ambos módulos (user y search) importen desde el paquete compartido.
// Aquí se re-exportan para backward compatibility dentro del módulo search.
package domain

import (
	sharedSearch "github.com/ProacTrip/Backend/internal/shared/search"
)

// SavedSearchProvider es un type alias para shared/search.SavedSearchProvider.
// Los consumidores internos del módulo search pueden seguir usando domain.SavedSearchProvider.
type SavedSearchProvider = sharedSearch.SavedSearchProvider

// SavedSearchData es un type alias para shared/search.SavedSearchData.
type SavedSearchData = sharedSearch.SavedSearchData
