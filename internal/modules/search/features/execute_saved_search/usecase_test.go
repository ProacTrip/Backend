// Tests unitarios para el caso de uso execute_saved_search.
// Cubre flight, hotel, ai, both, errores, detección de tipo legacy.
package execute_saved_search_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/execute_saved_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
)

// =============================================================================
// Stubs
// =============================================================================

type stubSavedSearchProvider struct {
	data *domain.SavedSearchData
	err  error
}

func (s *stubSavedSearchProvider) GetByID(ctx context.Context, searchID uuid.UUID) (*domain.SavedSearchData, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

type stubFlightSearcher struct {
	resp *search_flights.Response
	err  error
}

func (s *stubFlightSearcher) Execute(ctx context.Context, cmd search_flights.Command) (*search_flights.Response, error) {
	return s.resp, s.err
}

type stubHotelSearcher struct {
	resp *search_hotels.Response
	err  error
}

func (s *stubHotelSearcher) Execute(ctx context.Context, cmd search_hotels.Command) (*search_hotels.Response, error) {
	return s.resp, s.err
}

type stubAISearcher struct {
	resp *ai_search.Response
	err  error
}

func (s *stubAISearcher) Execute(ctx context.Context, cmd ai_search.Command, userID string) (*ai_search.Response, error) {
	return s.resp, s.err
}

// =============================================================================
// Helpers
// =============================================================================

func newStoredFlightSearch(userID uuid.UUID) *domain.SavedSearchData {
	params, _ := json.Marshal(search_flights.Command{
		TripType:     "round_trip",
		Departure:    "EZE",
		Arrival:      "MAD",
		OutboundDate: "2026-06-15",
		ReturnDate:   "2026-06-30",
		Adults:       1,
	})
	return &domain.SavedSearchData{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		SearchType: "flight",
		Parameters: params,
	}
}

func newStoredHotelSearch(userID uuid.UUID) *domain.SavedSearchData {
	params, _ := json.Marshal(search_hotels.Command{
		Query:        "Madrid",
		CheckInDate:  "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:       2,
	})
	return &domain.SavedSearchData{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		SearchType: "hotel",
		Parameters: params,
	}
}

func newStoredAISearch(userID uuid.UUID) *domain.SavedSearchData {
	params, _ := json.Marshal(ai_search.Command{
		Message: "Vuelos baratos a Madrid",
	})
	return &domain.SavedSearchData{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		SearchType: "ai",
		Parameters: params,
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestUseCase_Execute_Flight_Success(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	savedSearch := newStoredFlightSearch(userID)

	provider := &stubSavedSearchProvider{data: savedSearch}
	flightSearcher := &stubFlightSearcher{
		resp: &search_flights.Response{
			TripType:     "round_trip",
			ResultsState: "complete",
		},
	}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      flightSearcher,
		HotelSearcher:       &stubHotelSearcher{},
		AISearcher:          &stubAISearcher{},
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: savedSearch.ID,
		UserID:        userID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SearchType != "flight" {
		t.Errorf("SearchType = %q, want %q", resp.SearchType, "flight")
	}
	if resp.Results.Flights == nil {
		t.Error("expected non-nil Flights in response")
	}
}

func TestUseCase_Execute_Hotel_Success(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	savedSearch := newStoredHotelSearch(userID)

	provider := &stubSavedSearchProvider{data: savedSearch}
	hotelSearcher := &stubHotelSearcher{
		resp: &search_hotels.Response{
			Type:         "hotel_search",
			ResultsState: "complete",
		},
	}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      &stubFlightSearcher{},
		HotelSearcher:       hotelSearcher,
		AISearcher:          &stubAISearcher{},
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: savedSearch.ID,
		UserID:        userID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SearchType != "hotel" {
		t.Errorf("SearchType = %q, want %q", resp.SearchType, "hotel")
	}
	if resp.Results.Hotels == nil {
		t.Error("expected non-nil Hotels in response")
	}
}

func TestUseCase_Execute_AI_Success(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	savedSearch := newStoredAISearch(userID)

	provider := &stubSavedSearchProvider{data: savedSearch}
	aiSearcher := &stubAISearcher{
		resp: &ai_search.Response{
			Intent: "flight",
		},
	}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      &stubFlightSearcher{},
		HotelSearcher:       &stubHotelSearcher{},
		AISearcher:          aiSearcher,
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: savedSearch.ID,
		UserID:        userID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SearchType != "ai" {
		t.Errorf("SearchType = %q, want %q", resp.SearchType, "ai")
	}
	if resp.Results.AIResponse == nil {
		t.Error("expected non-nil AIResponse in response")
	}
}

func TestUseCase_Execute_Both_Success(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	params, _ := json.Marshal(map[string]string{
		"origin":      "EZE",
		"destination": "MAD",
		"check_in":    "2026-06-15",
		"check_out":   "2026-06-20",
	})
	savedSearch := &domain.SavedSearchData{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		SearchType: "both",
		Parameters: params,
	}

	provider := &stubSavedSearchProvider{data: savedSearch}
	flightSearcher := &stubFlightSearcher{
		resp: &search_flights.Response{
			TripType:     "round_trip",
			ResultsState: "complete",
		},
	}
	hotelSearcher := &stubHotelSearcher{
		resp: &search_hotels.Response{
			Type:         "hotel_search",
			ResultsState: "complete",
		},
	}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      flightSearcher,
		HotelSearcher:       hotelSearcher,
		AISearcher:          &stubAISearcher{},
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: savedSearch.ID,
		UserID:        userID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SearchType != "both" {
		t.Errorf("SearchType = %q, want %q", resp.SearchType, "both")
	}
	if resp.Results.Flights == nil {
		t.Error("expected non-nil Flights for 'both' search")
	}
	if resp.Results.Hotels == nil {
		t.Error("expected non-nil Hotels for 'both' search")
	}
}

func TestUseCase_Execute_Both_FlightFailsHotelSucceeds(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	params, _ := json.Marshal(map[string]string{
		"origin":      "EZE",
		"destination": "MAD",
	})
	savedSearch := &domain.SavedSearchData{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		SearchType: "both",
		Parameters: params,
	}

	provider := &stubSavedSearchProvider{data: savedSearch}
	flightSearcher := &stubFlightSearcher{
		err: errors.New("flights unavailable"),
	}
	hotelSearcher := &stubHotelSearcher{
		resp: &search_hotels.Response{
			Type:         "hotel_search",
			ResultsState: "complete",
		},
	}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      flightSearcher,
		HotelSearcher:       hotelSearcher,
		AISearcher:          &stubAISearcher{},
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: savedSearch.ID,
		UserID:        userID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error with partial failure: %v", err)
	}
	if resp.Results.FlightsError == "" {
		t.Error("expected FlightsError when flight search fails in 'both' mode")
	}
	if resp.Results.Hotels == nil {
		t.Error("expected Hotels when hotel search succeeds in 'both' mode")
	}
}

func TestUseCase_Execute_Both_AllFail(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	savedSearch := &domain.SavedSearchData{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		SearchType: "both",
		Parameters: json.RawMessage(`{}`),
	}

	provider := &stubSavedSearchProvider{data: savedSearch}
	flightSearcher := &stubFlightSearcher{
		err: errors.New("flights down"),
	}
	hotelSearcher := &stubHotelSearcher{
		err: errors.New("hotels down"),
	}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      flightSearcher,
		HotelSearcher:       hotelSearcher,
		AISearcher:          &stubAISearcher{},
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: savedSearch.ID,
		UserID:        userID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error when both searchers fail in 'both' mode")
	}
}

func TestUseCase_Execute_WrongUser_ReturnsError(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	wrongUserID := uuid.Must(uuid.NewV7())

	savedSearch := newStoredFlightSearch(userID)
	provider := &stubSavedSearchProvider{data: savedSearch}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      &stubFlightSearcher{},
		HotelSearcher:       &stubHotelSearcher{},
		AISearcher:          &stubAISearcher{},
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: savedSearch.ID,
		UserID:        wrongUserID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error when userID doesn't match saved search owner")
	}
	if !errors.Is(err, domain.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got: %v", err)
	}
}

func TestUseCase_Execute_ProviderNotFound(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	provider := &stubSavedSearchProvider{
		err: errors.New("saved search not found"),
	}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      &stubFlightSearcher{},
		HotelSearcher:       &stubHotelSearcher{},
		AISearcher:          &stubAISearcher{},
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: uuid.Must(uuid.NewV7()),
		UserID:        userID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error when saved search is not found")
	}
}

func TestCommand_Validate_EmptySavedSearchID(t *testing.T) {
	cmd := execute_saved_search.Command{
		SavedSearchID: uuid.Nil,
		UserID:        uuid.Must(uuid.NewV7()),
	}
	err := cmd.Validate()
	if err == nil {
		t.Error("expected error for nil SavedSearchID")
	}
}

func TestCommand_Validate_EmptyUserID(t *testing.T) {
	cmd := execute_saved_search.Command{
		SavedSearchID: uuid.Must(uuid.NewV7()),
		UserID:        uuid.Nil,
	}
	err := cmd.Validate()
	if err == nil {
		t.Error("expected error for nil UserID")
	}
}

// Test for legacy search type detection (empty SearchType with params)
func TestUseCase_Execute_LegacyFlightDetection(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	params, _ := json.Marshal(map[string]string{
		"origin":      "EZE",
		"destination": "MAD",
	})
	savedSearch := &domain.SavedSearchData{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		SearchType: "", // legacy: empty type
		Parameters: params,
	}

	provider := &stubSavedSearchProvider{data: savedSearch}
	flightSearcher := &stubFlightSearcher{
		resp: &search_flights.Response{
			TripType: "round_trip",
		},
	}

	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      flightSearcher,
		HotelSearcher:       &stubHotelSearcher{},
		AISearcher:          &stubAISearcher{},
	})

	cmd := execute_saved_search.Command{
		SavedSearchID: savedSearch.ID,
		UserID:        userID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error for legacy search: %v", err)
	}
	if resp.SearchType != "flight" {
		t.Errorf("SearchType = %q, want %q for legacy origin/destination params", resp.SearchType, "flight")
	}
}
