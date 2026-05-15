// CandidateSource implementations para el pipeline de discovery.
//
// Define la interfaz CandidateSource y 2 implementaciones basadas en user data:
//   - FavoritesSource: extrae destinos de los favoritos del usuario
//   - SavedSearchSource: extrae destinos de parámetros de búsquedas guardadas
//
// Las fuentes se ejecutan en cadena: favorites → saved.
// La deduplicación por destino se maneja en el pipeline.
package ai_search

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// CandidateSource interface — AR-005
// =============================================================================

// CandidateSource genera candidatos de destinos para el pipeline de discovery.
// Cada implementación representa una fuente distinta de candidatos
// (favoritos, búsquedas guardadas, popularidad, temporada).
type CandidateSource interface {
	// Name devuelve el nombre de la fuente (ej. "favorites", "popularity").
	Name() string
	// Generate genera candidatos a partir del contexto de recomendación.
	Generate(ctx context.Context, rc *RecommendationContext) ([]Candidate, error)
}

// =============================================================================
// FavoritesSource — AR-006
// =============================================================================

// FavoritesSource genera candidatos a partir de los favoritos del usuario.
// Extrae los destinos de los parámetros de búsquedas guardadas marcadas como favoritas.
type FavoritesSource struct {
	// getFavorites es una función que obtiene los favoritos del usuario.
	// Se expone como campo para permitir mock en tests.
	getFavorites func(userID string) []domain.SavedSearchData
}

// Name devuelve el nombre de la fuente.
func (s *FavoritesSource) Name() string { return "favorites" }

// Generate extrae destinos de los favoritos del usuario.
// Si el usuario no tiene favoritos, devuelve slice vacío (no error).
func (s *FavoritesSource) Generate(ctx context.Context, rc *RecommendationContext) ([]Candidate, error) {
	if s.getFavorites == nil {
		slog.DebugContext(ctx, "FavoritesSource: getFavorites no configurado, omitiendo")
		return nil, nil
	}

	favs := s.getFavorites(rc.UserID)
	if len(favs) == 0 {
		return nil, nil
	}

	cands := make([]Candidate, 0, len(favs))
	for _, f := range favs {
		c := extractDestinationFromParams(f.Parameters)
		if c.Destination == "" {
			continue
		}
		c.Source = "user_favorite"
		cands = append(cands, c)
	}

	return cands, nil
}

// =============================================================================
// SavedSearchSource — AR-007
// =============================================================================

// SavedSearchSource genera candidatos a partir de las búsquedas guardadas del usuario.
// Extrae destinos de los parámetros JSON de cada búsqueda guardada.
type SavedSearchSource struct {
	// getSavedSearches es una función que obtiene las búsquedas guardadas del usuario.
	getSavedSearches func(userID string) []domain.SavedSearchData
}

// Name devuelve el nombre de la fuente.
func (s *SavedSearchSource) Name() string { return "saved_searches" }

// Generate extrae destinos de las búsquedas guardadas del usuario.
// Usuarios anónimos (UserID vacío) devuelven slice vacío.
func (s *SavedSearchSource) Generate(ctx context.Context, rc *RecommendationContext) ([]Candidate, error) {
	if rc.UserID == "" {
		return nil, nil // usuario anónimo no tiene búsquedas guardadas
	}
	if s.getSavedSearches == nil {
		slog.DebugContext(ctx, "SavedSearchSource: getSavedSearches no configurado, omitiendo")
		return nil, nil
	}

	searches := s.getSavedSearches(rc.UserID)
	if len(searches) == 0 {
		return nil, nil
	}

	cands := make([]Candidate, 0, len(searches))
	for _, ss := range searches {
		c := extractDestinationFromParams(ss.Parameters)
		if c.Destination == "" {
			continue
		}
		c.Source = "user_saved"
		cands = append(cands, c)
	}

	return cands, nil
}

// =============================================================================
// Helpers
// =============================================================================

// extractDestinationFromParams extrae un Candidate de los parámetros JSON
// de una búsqueda guardada o favorito.
func extractDestinationFromParams(params json.RawMessage) Candidate {
	if len(params) == 0 {
		return Candidate{}
	}

	var p struct {
		Destination string `json:"destination"`
		Country     string `json:"country"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Candidate{}
	}

	return Candidate{
		Destination: p.Destination,
		Country:     p.Country,
	}
}
