// Adapter que implementa shared/search.SavedSearchProvider usando el repositorio
// de búsquedas guardadas del módulo user. Resuelve búsquedas guardadas
// para el módulo search sin crear un acoplamiento circular.
package adapters

import (
	"context"

	"github.com/google/uuid"

	sharedSearch "github.com/ProacTrip/Backend/internal/shared/search"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// Compile-time interface check
var _ sharedSearch.SavedSearchProvider = (*SearchResolver)(nil)

// SearchResolver adapts the user module's SavedSearchRepository to
// the search module's SavedSearchProvider interface.
type SearchResolver struct {
	repo domain.SavedSearchRepository
}

// NewSearchResolver creates a new SearchResolver backed by the given repository.
func NewSearchResolver(repo domain.SavedSearchRepository) *SearchResolver {
	return &SearchResolver{repo: repo}
}

// GetByID retrieves a saved search by ID and converts it to the search module's format.
func (r *SearchResolver) GetByID(ctx context.Context, searchID uuid.UUID) (*sharedSearch.SavedSearchData, error) {
	ss, err := r.repo.GetByID(ctx, searchID)
	if err != nil {
		return nil, err
	}

	searchType := ""
	if ss.SearchType != nil {
		searchType = *ss.SearchType
	}

	name := ""
	if ss.Name != nil {
		name = *ss.Name
	}

	return &sharedSearch.SavedSearchData{
		ID:                ss.ID,
		UserID:            ss.UserID,
		Name:              name,
		Parameters:        ss.Parameters,
		Filters:           ss.Filters,
		SearchType:        searchType,
		ParametersVersion: ss.ParametersVersion,
	}, nil
}
