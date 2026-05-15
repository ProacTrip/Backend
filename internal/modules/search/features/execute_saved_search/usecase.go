// Caso de uso: Ejecutar búsqueda guardada (POST /v1/search/execute_saved).
// Orquesta la ejecución de búsquedas guardadas usando los searchers del módulo search.
package execute_saved_search

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
)

// =============================================================================
// Ports
// =============================================================================

// FlightSearcher executes flight searches.
type FlightSearcher interface {
	Execute(ctx context.Context, cmd search_flights.Command) (*search_flights.Response, error)
}

// HotelSearcher executes hotel searches.
type HotelSearcher interface {
	Execute(ctx context.Context, cmd search_hotels.Command) (*search_hotels.Response, error)
}

// AISearcher executes AI-powered unified searches.
type AISearcher interface {
	Execute(ctx context.Context, cmd ai_search.Command, userID string) (*ai_search.Response, error)
}

// =============================================================================
// UseCase
// =============================================================================

type UseCaseDeps struct {
	SavedSearchProvider domain.SavedSearchProvider
	FlightSearcher      FlightSearcher
	HotelSearcher       HotelSearcher
	AISearcher          AISearcher
}

type UseCase struct {
	savedSearchProvider domain.SavedSearchProvider
	flightSearcher      FlightSearcher
	hotelSearcher       HotelSearcher
	aiSearcher          AISearcher
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		savedSearchProvider: deps.SavedSearchProvider,
		flightSearcher:      deps.FlightSearcher,
		hotelSearcher:       deps.HotelSearcher,
		aiSearcher:          deps.AISearcher,
	}
}

// Execute ejecuta una búsqueda guardada por su ID.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 1. Obtener la búsqueda guardada
	savedSearch, err := uc.savedSearchProvider.GetByID(ctx, cmd.SavedSearchID)
	if err != nil {
		return nil, fmt.Errorf("get saved search: %w", err)
	}

	// 2. Verificar ownership
	if savedSearch.UserID != cmd.UserID {
		return nil, domain.ErrTokenInvalid
	}

	// 3. Determinar search type
	searchType := defaultedSearchType(savedSearch.SearchType, savedSearch.Parameters)

	// 4. Ejecutar según el tipo
	resp := &Response{
		SearchType: searchType,
		SearchID:   cmd.SavedSearchID.String(),
	}

	switch searchType {
	case "flight":
		flightResp, err := uc.executeFlight(ctx, savedSearch.Parameters)
		if err != nil {
			return nil, fmt.Errorf("flight search: %w", err)
		}
		flightsJSON, _ := json.Marshal(flightResp)
		resp.Results.Flights = flightsJSON

	case "hotel":
		hotelResp, err := uc.executeHotel(ctx, savedSearch.Parameters)
		if err != nil {
			return nil, fmt.Errorf("hotel search: %w", err)
		}
		hotelsJSON, _ := json.Marshal(hotelResp)
		resp.Results.Hotels = hotelsJSON

	case "ai":
		aiResp, err := uc.executeAI(ctx, savedSearch.Parameters, savedSearch.UserID.String())
		if err != nil {
			return nil, fmt.Errorf("ai search: %w", err)
		}
		aiJSON, _ := json.Marshal(aiResp)
		resp.Results.AIResponse = aiJSON

	case "both":
		var flightResp *search_flights.Response
		var hotelResp *search_hotels.Response
		var flightErr, hotelErr error

		g := new(errgroup.Group)
		g.Go(func() error {
			flightResp, flightErr = uc.executeFlight(ctx, savedSearch.Parameters)
			return nil
		})
		g.Go(func() error {
			hotelResp, hotelErr = uc.executeHotel(ctx, savedSearch.Parameters)
			return nil
		})
		g.Wait()

		if flightErr != nil {
			resp.Results.FlightsError = flightErr.Error()
		} else if flightResp != nil {
			flightsJSON, _ := json.Marshal(flightResp)
			resp.Results.Flights = flightsJSON
		}

		if hotelErr != nil {
			resp.Results.HotelsError = hotelErr.Error()
		} else if hotelResp != nil {
			hotelsJSON, _ := json.Marshal(hotelResp)
			resp.Results.Hotels = hotelsJSON
		}

		if flightErr != nil && hotelErr != nil {
			return nil, fmt.Errorf("%w: flights: %w | hotels: %w",
				domain.ErrProviderUnavailable, flightErr, hotelErr)
		}
	}

	return resp, nil
}

// executeFlight unmarshals parameters into a flight command and executes it.
func (uc *UseCase) executeFlight(ctx context.Context, params json.RawMessage) (*search_flights.Response, error) {
	var cmd search_flights.Command
	if err := json.Unmarshal(params, &cmd); err != nil {
		return nil, fmt.Errorf("parse flight parameters: %w", err)
	}
	return uc.flightSearcher.Execute(ctx, cmd)
}

// executeHotel unmarshals parameters into a hotel command and executes it.
func (uc *UseCase) executeHotel(ctx context.Context, params json.RawMessage) (*search_hotels.Response, error) {
	var cmd search_hotels.Command
	if err := json.Unmarshal(params, &cmd); err != nil {
		return nil, fmt.Errorf("parse hotel parameters: %w", err)
	}
	return uc.hotelSearcher.Execute(ctx, cmd)
}

// executeAI unmarshals parameters into an AI command and executes it.
func (uc *UseCase) executeAI(ctx context.Context, params json.RawMessage, userID string) (*ai_search.Response, error) {
	var cmd ai_search.Command
	if err := json.Unmarshal(params, &cmd); err != nil {
		return nil, fmt.Errorf("parse ai parameters: %w", err)
	}
	return uc.aiSearcher.Execute(ctx, cmd, userID)
}

// =============================================================================
// Search type detection (legacy support)
// =============================================================================

// searchParams is a flexible struct for detecting search type from parameters.
type searchParams struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	CheckIn     string `json:"check_in"`
	CheckOut    string `json:"check_out"`
	Message     string `json:"message"`
}

// defaultedSearchType returns the effective search type, detecting it from
// parameters structure when the stored type is empty (legacy searches).
func defaultedSearchType(storedType string, params json.RawMessage) string {
	if storedType != "" {
		return storedType
	}

	// Legacy: detect from parameters structure
	var p searchParams
	if err := json.Unmarshal(params, &p); err != nil {
		return "flight" // safe fallback
	}

	// Has origin/destination → flight
	if p.Origin != "" || p.Destination != "" {
		return "flight"
	}

	// Has check_in → hotel
	if p.CheckIn != "" || p.CheckOut != "" {
		return "hotel"
	}

	// Has message → ai
	if p.Message != "" {
		return "ai"
	}

	return "flight" // safe fallback
}
