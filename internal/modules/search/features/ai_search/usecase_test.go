package ai_search_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
)

// =============================================================================
// Stub implementations for ai_search use case dependencies
// =============================================================================

// stubInterpreter implements domain.AIInterpreter with configurable responses.
type stubInterpreter struct {
	intent *domain.TravelIntent
	err    error
}

func (s *stubInterpreter) Parse(ctx context.Context, message string, history []domain.ConversationMessage, language string) (*domain.TravelIntent, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.intent, nil
}

// stubFlightSearcher implements FlightSearcher for testing.
type stubFlightSearcher struct {
	called  bool
	lastCmd search_flights.Command
	resp    *search_flights.Response
	err     error
}

func (s *stubFlightSearcher) Execute(ctx context.Context, cmd search_flights.Command) (*search_flights.Response, error) {
	s.called = true
	s.lastCmd = cmd
	if s.err != nil {
		return nil, s.err
	}
	if s.resp != nil {
		return s.resp, nil
	}
	return &search_flights.Response{}, nil
}

// stubHotelSearcher implements HotelSearcher for testing.
type stubHotelSearcher struct {
	called  bool
	lastCmd search_hotels.Command
	resp    *search_hotels.Response
	err     error
}

func (s *stubHotelSearcher) Execute(ctx context.Context, cmd search_hotels.Command) (*search_hotels.Response, error) {
	s.called = true
	s.lastCmd = cmd
	if s.err != nil {
		return nil, s.err
	}
	if s.resp != nil {
		return s.resp, nil
	}
	return &search_hotels.Response{}, nil
}

// stubConversationStore implements conversationStore for testing.
type stubConversationStore struct {
	convs map[string]*domain.ConversationState
}

func (s *stubConversationStore) GetConversation(ctx context.Context, id string) (*domain.ConversationState, error) {
	if s.convs == nil {
		return nil, nil
	}
	return s.convs[id], nil
}

func (s *stubConversationStore) SaveConversation(ctx context.Context, conv *domain.ConversationState) error {
	if s.convs == nil {
		s.convs = make(map[string]*domain.ConversationState)
	}
	s.convs[conv.ID] = conv
	return nil
}

// New ConversationStore methods — stubbed for tests.
func (s *stubConversationStore) Save(ctx context.Context, conv *ai_search.Conversation) error {
	return nil
}
func (s *stubConversationStore) Load(ctx context.Context, convID string) (*ai_search.Conversation, error) {
	return nil, nil
}
func (s *stubConversationStore) Delete(ctx context.Context, convID, userID string) error {
	return nil
}
func (s *stubConversationStore) ListUserConversations(ctx context.Context, userID string) ([]ai_search.ConversationPreview, error) {
	return nil, nil
}
func (s *stubConversationStore) ResetTTL(ctx context.Context, convID string) error {
	return nil
}

// =============================================================================
// Helper: valid flight intent
// =============================================================================

func newFlightsIntent() *domain.TravelIntent {
	return &domain.TravelIntent{
		Type:       "flights",
		Confidence: 0.95,
		FlightParams: &domain.FlightSearchRequest{
			Departure:    "EZE",
			Arrival:      "MAD",
			OutboundDate: "2026-06-15",
			ReturnDate:   "2026-06-30",
			Adults:       1,
			TripType:     "round_trip",
		},
	}
}

func newHotelsIntent() *domain.TravelIntent {
	return &domain.TravelIntent{
		Type:       "hotels",
		Confidence: 0.90,
		HotelParams: &domain.HotelSearchRequest{
			Query:        "Barcelona",
			CheckInDate:  "2026-07-01",
			CheckOutDate: "2026-07-05",
			Adults:       2,
		},
	}
}

func newBothIntent() *domain.TravelIntent {
	return &domain.TravelIntent{
		Type:       "both",
		Confidence: 0.85,
		FlightParams: &domain.FlightSearchRequest{
			Departure:    "EZE",
			Arrival:      "BCN",
			OutboundDate: "2026-07-01",
			ReturnDate:   "2026-07-10",
			Adults:       2,
			TripType:     "round_trip",
		},
		HotelParams: &domain.HotelSearchRequest{
			Query:        "Barcelona",
			CheckInDate:  "2026-07-01",
			CheckOutDate: "2026-07-10",
			Adults:       2,
		},
	}
}

func newIncompleteIntent() *domain.TravelIntent {
	return &domain.TravelIntent{
		Type:          "incomplete",
		Confidence:    0.5,
		MissingFields: []string{"outbound_date", "destination"},
		FollowUp:      "¿Desde qué ciudad salís y en qué fecha?",
	}
}

func newAmbiguousIntent() *domain.TravelIntent {
	return &domain.TravelIntent{
		Type:       "ambiguous",
		Confidence: 0.3,
		FollowUp:   "¿Estás buscando vuelos, hoteles, o ambos?",
	}
}

// =============================================================================
// UseCase Tests
// =============================================================================

// TestUseCase_IncompleteIntent verifies that when the AI returns an "incomplete"
// intent, the use case returns a follow-up response WITHOUT executing any search.
func TestUseCase_IncompleteIntent(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newIncompleteIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Quiero viajar"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error for incomplete intent, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Intent != "incomplete" {
		t.Errorf("expected intent 'incomplete', got %q", resp.Intent)
	}
	if resp.Message == "" {
		t.Error("expected follow-up message, got empty")
	}
	if len(resp.MissingFields) == 0 {
		t.Error("expected missing fields to be populated")
	}
	// Verify no search was performed
	if flightsSearcher.called {
		t.Error("flights search should NOT be called for incomplete intent")
	}
	if hotelsSearcher.called {
		t.Error("hotels search should NOT be called for incomplete intent")
	}
}

// TestUseCase_FlightsIntent verifies that a "flights" intent triggers the flights
// use case and returns flight results.
func TestUseCase_FlightsIntent(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Busco vuelos de Buenos Aires a Madrid"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error for flights intent, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Intent != "flights" {
		t.Errorf("expected intent 'flights', got %q", resp.Intent)
	}
	// Verify flights was called
	if !flightsSearcher.called {
		t.Error("flights search should be called for flights intent")
	}
	// Verify hotels was NOT called
	if hotelsSearcher.called {
		t.Error("hotels search should NOT be called for flights intent")
	}
}

// TestUseCase_HotelsIntent verifies that a "hotels" intent triggers the hotels
// use case and returns hotel results.
func TestUseCase_HotelsIntent(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newHotelsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Hoteles en Barcelona para Julio"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error for hotels intent, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Intent != "hotels" {
		t.Errorf("expected intent 'hotels', got %q", resp.Intent)
	}
	if !hotelsSearcher.called {
		t.Error("hotels search should be called for hotels intent")
	}
	if flightsSearcher.called {
		t.Error("flights search should NOT be called for hotels intent")
	}
}

// TestUseCase_BothIntent verifies that a "both" intent triggers BOTH the flights
// and hotels use cases in parallel.
func TestUseCase_BothIntent(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newBothIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Viaje a Barcelona con vuelo y hotel"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error for both intent, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Intent != "both" {
		t.Errorf("expected intent 'both', got %q", resp.Intent)
	}
	// Verify both searchers were called
	if !flightsSearcher.called {
		t.Error("flights search should be called for both intent")
	}
	if !hotelsSearcher.called {
		t.Error("hotels search should be called for both intent")
	}
}

// TestUseCase_AmbiguousIntent verifies that an "ambiguous" intent returns
// a clarification question without executing any search.
func TestUseCase_AmbiguousIntent(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newAmbiguousIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Quiero viajar"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error for ambiguous intent, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Intent != "ambiguous" {
		t.Errorf("expected intent 'ambiguous', got %q", resp.Intent)
	}
	if resp.Message == "" {
		t.Error("expected clarification message, got empty")
	}
	if flightsSearcher.called {
		t.Error("flights search should NOT be called for ambiguous intent")
	}
	if hotelsSearcher.called {
		t.Error("hotels search should NOT be called for ambiguous intent")
	}
}

// TestUseCase_TurnLimitExceeded verifies that when the conversation exceeds
// the max turns, the use case returns ErrTurnLimitExceeded.
func TestUseCase_TurnLimitExceeded(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}
	// Pre-populate a conversation with 5 turns (anon max)
	convStore.convs = map[string]*domain.ConversationState{
		"conv_full": {
			ID:        "conv_full",
			UserID:    "",
			TurnCount: 5,
			MaxTurns:  5,
		},
	}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{
		Message:        "Más opciones",
		ConversationID: "conv_full",
	}

	_, err := uc.Execute(ctx, cmd, "")

	if err == nil {
		t.Fatal("expected turn limit error, got nil")
	}
	if !errors.Is(err, domain.ErrTurnLimitExceeded) {
		t.Errorf("expected ErrTurnLimitExceeded, got: %v", err)
	}
	// Verify no search was performed
	if flightsSearcher.called {
		t.Error("flights search should NOT be called when turn limit exceeded")
	}
}

// TestUseCase_AIUnavailable verifies that when the AI interpreter returns an
// error, the use case returns ErrAIUnavailable.
func TestUseCase_AIUnavailable(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{
		err: errors.New("connection refused"),
	}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Vuelos baratos"}
	_, err := uc.Execute(ctx, cmd, "")

	if err == nil {
		t.Fatal("expected error when AI is unavailable, got nil")
	}
	// Verify no search was performed
	if flightsSearcher.called {
		t.Error("flights search should NOT be called when AI fails")
	}
}

// TestUseCase_BothIntent_PartialFlightFailure verifies that when the "both"
// intent fires both searchers in parallel and flights fails while hotels
// succeeds, the use case returns a partial response with error markers.
func TestUseCase_BothIntent_PartialFlightFailure(t *testing.T) {
	ctx := t.Context()

	flightErr := errors.New("serpapi rate limited")
	interpreter := &stubInterpreter{intent: newBothIntent()}
	flightsSearcher := &stubFlightSearcher{err: flightErr}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Viaje completo a Barcelona"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no fatal error on partial failure, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Intent != "both" {
		t.Errorf("expected intent 'both', got %q", resp.Intent)
	}

	// Verify both searchers were called
	if !flightsSearcher.called {
		t.Error("flights search should be called for both intent (even if it fails)")
	}
	if !hotelsSearcher.called {
		t.Error("hotels search should be called for both intent")
	}

	// Verify error marker for failed flights
	if resp.FlightsError == "" {
		t.Error("expected flights_error to be set when flights fails")
	} else if resp.FlightsError != flightErr.Error() {
		t.Errorf("expected flights_error %q, got %q", flightErr.Error(), resp.FlightsError)
	}

	// Verify hotels result is present (successful)
	if resp.Hotels == nil {
		t.Error("expected hotels in response (hotels succeeded)")
	}

	// Verify no error marker for successful hotels
	if resp.HotelsError != "" {
		t.Errorf("expected no hotels_error, got %q", resp.HotelsError)
	}

	// Verify flights result is nil (failed)
	if resp.Flights != nil {
		t.Error("expected flights to be nil (failed)")
	}
}

// TestUseCase_BothIntent_PartialHotelFailure verifies that when the "both"
// intent fires both searchers in parallel and hotels fails while flights
// succeeds, the use case returns a partial response with error markers.
func TestUseCase_BothIntent_PartialHotelFailure(t *testing.T) {
	ctx := t.Context()

	hotelErr := errors.New("hotel provider timeout")
	interpreter := &stubInterpreter{intent: newBothIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{err: hotelErr}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Viaje completo a Barcelona"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no fatal error on partial failure, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Verify error marker for failed hotels
	if resp.HotelsError == "" {
		t.Error("expected hotels_error to be set when hotels fails")
	} else if resp.HotelsError != hotelErr.Error() {
		t.Errorf("expected hotels_error %q, got %q", hotelErr.Error(), resp.HotelsError)
	}

	// Verify flights result is present (successful)
	if resp.Flights == nil {
		t.Error("expected flights in response (flights succeeded)")
	}

	// Verify no error marker for successful flights
	if resp.FlightsError != "" {
		t.Errorf("expected no flights_error, got %q", resp.FlightsError)
	}

	// Verify hotels result is nil (failed)
	if resp.Hotels != nil {
		t.Error("expected hotels to be nil (failed)")
	}
}

// TestUseCase_BothIntent_BothFailures verifies that when BOTH searchers fail,
// the use case returns ErrProviderUnavailable (502 Bad Gateway) instead of 500.
func TestUseCase_BothIntent_BothFailures(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newBothIntent()}
	flightsSearcher := &stubFlightSearcher{err: errors.New("flights down")}
	hotelsSearcher := &stubHotelSearcher{err: errors.New("hotels down")}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Viaje completo a Barcelona"}
	_, err := uc.Execute(ctx, cmd, "")

	if err == nil {
		t.Fatal("expected fatal error when both searchers fail, got nil")
	}
	if !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable (502), got: %v", err)
	}
}

// =============================================================================
// W2 — Filter propagation tests (R4)
// =============================================================================

// TestUseCase_FilterPropagation_Flights verifies that FilterCriteria fields
// (MaxPrice, Stops) from the TravelIntent are propagated to the flights command
// built by buildFlightCommand.
func TestUseCase_FilterPropagation_Flights(t *testing.T) {
	ctx := t.Context()

	maxPrice := 500.0
	stops := "nonstop"
	intent := &domain.TravelIntent{
		Type:       "flights",
		Confidence: 0.90,
		FlightParams: &domain.FlightSearchRequest{
			Departure:    "EZE",
			Arrival:      "MAD",
			OutboundDate: "2026-06-15",
			ReturnDate:   "2026-06-30",
			Adults:       1,
			TripType:     "round_trip",
			MaxPrice:     &maxPrice,
			Stops:        stops,
		},
	}

	interpreter := &stubInterpreter{intent: intent}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Vuelos sin escalas hasta 500 USD"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Verify filter fields were propagated to the flights command
	if flightsSearcher.lastCmd.MaxPrice == nil {
		t.Error("expected MaxPrice to be propagated to flights command")
	} else if *flightsSearcher.lastCmd.MaxPrice != maxPrice {
		t.Errorf("MaxPrice = %v, want %v", *flightsSearcher.lastCmd.MaxPrice, maxPrice)
	}
	if flightsSearcher.lastCmd.Stops != stops {
		t.Errorf("Stops = %q, want %q", flightsSearcher.lastCmd.Stops, stops)
	}
}

// TestUseCase_FilterPropagation_Hotels verifies that FilterCriteria fields
// (Rating, MaxPrice) from the TravelIntent are propagated to the hotels command
// built by buildHotelCommand.
func TestUseCase_FilterPropagation_Hotels(t *testing.T) {
	ctx := t.Context()

	maxPrice := 300.0
	rating := 4
	freeCancel := true

	intent := &domain.TravelIntent{
		Type:       "hotels",
		Confidence: 0.88,
		HotelParams: &domain.HotelSearchRequest{
			Query:            "Barcelona",
			CheckInDate:      "2026-07-01",
			CheckOutDate:     "2026-07-05",
			Adults:           2,
			MaxPrice:         &maxPrice,
			Rating:           &rating,
			FreeCancellation: freeCancel,
		},
	}

	interpreter := &stubInterpreter{intent: intent}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Hotel barato 4 estrellas en Barcelona"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Verify filter fields were propagated to the hotels command
	if hotelsSearcher.lastCmd.MaxPrice == nil {
		t.Error("expected MaxPrice to be propagated to hotels command")
	} else if *hotelsSearcher.lastCmd.MaxPrice != maxPrice {
		t.Errorf("MaxPrice = %v, want %v", *hotelsSearcher.lastCmd.MaxPrice, maxPrice)
	}
	if hotelsSearcher.lastCmd.Rating == nil {
		t.Error("expected Rating to be propagated to hotels command")
	} else if *hotelsSearcher.lastCmd.Rating != rating {
		t.Errorf("Rating = %d, want %d", *hotelsSearcher.lastCmd.Rating, rating)
	}
	if hotelsSearcher.lastCmd.FreeCancellation != freeCancel {
		t.Errorf("FreeCancellation = %v, want %v", hotelsSearcher.lastCmd.FreeCancellation, freeCancel)
	}
}

// TestUseCase_NoFilter_WhenNil verifies that when FlightParams are nil,
// filters in the command are set to defaults (nil/zero) without panicking.
func TestUseCase_NoFilter_WhenNil(t *testing.T) {
	ctx := t.Context()

	// Intent without any params — only type and confidence
	intent := &domain.TravelIntent{
		Type:       "flights",
		Confidence: 0.95,
	}

	interpreter := &stubInterpreter{intent: intent}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "vuelo"}
	_, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error with nil params, got: %v", err)
	}

	// Verify defaults were applied (not nil filters)
	if flightsSearcher.lastCmd.Adults != 1 {
		t.Errorf("expected default Adults=1, got %d", flightsSearcher.lastCmd.Adults)
	}
	// MaxPrice should be nil (not set when no params)
	if flightsSearcher.lastCmd.MaxPrice != nil {
		t.Error("expected MaxPrice to be nil when not in intent")
	}
}

// =============================================================================
// W3 — TTL expiry test (R6)
// =============================================================================

// expiringConvStore simulates a conversation store where the first lookup
// returns nil (expired/missing) and a subsequent save creates a new one.
type expiringConvStore struct {
	saved *domain.ConversationState
}

func (s *expiringConvStore) GetConversation(ctx context.Context, id string) (*domain.ConversationState, error) {
	return nil, nil // always "expired" / not found
}

func (s *expiringConvStore) SaveConversation(ctx context.Context, conv *domain.ConversationState) error {
	s.saved = conv
	return nil
}

// New ConversationStore methods — stubbed.
func (s *expiringConvStore) Save(ctx context.Context, conv *ai_search.Conversation) error { return nil }
func (s *expiringConvStore) Load(ctx context.Context, convID string) (*ai_search.Conversation, error) {
	return nil, nil
}
func (s *expiringConvStore) Delete(ctx context.Context, convID, userID string) error { return nil }
func (s *expiringConvStore) ListUserConversations(ctx context.Context, userID string) ([]ai_search.ConversationPreview, error) {
	return nil, nil
}
func (s *expiringConvStore) ResetTTL(ctx context.Context, convID string) error { return nil }

// TestUseCase_ConversationNotFound_ReturnsError verifies that when a conversation
// ID is provided but not found (expired or never existed), the use case returns
// ErrConversationNotFound instead of silently creating a new one.
func TestUseCase_ConversationNotFound_ReturnsError(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &expiringConvStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	// Pass an old (expired/non-existent) conversation ID
	cmd := ai_search.Command{
		Message:        "Vuelos a Madrid",
		ConversationID: "expired-conv-999",
	}

	_, err := uc.Execute(ctx, cmd, "")

	if err == nil {
		t.Fatal("expected ErrConversationNotFound for non-existent conversation ID, got nil")
	}
	if !errors.Is(err, domain.ErrConversationNotFound) {
		t.Errorf("expected ErrConversationNotFound, got: %v", err)
	}

	// Verify no search was performed
	if flightsSearcher.called {
		t.Error("flights search should NOT be called when conversation not found")
	}
}

// TestUseCase_EmptyConversationID_CreatesNew verifies that an empty conversation_id
// (first request) creates a new conversation.
func TestUseCase_EmptyConversationID_CreatesNew(t *testing.T) {
	ctx := t.Context()

	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Vuelos a Madrid"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error for new conversation, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.ConversationID == "" {
		t.Error("expected non-empty conversation_id for new conversation")
	}
	if resp.TurnCount != 1 {
		t.Errorf("expected turn_count 1, got %d", resp.TurnCount)
	}
}

// TestUseCase_ExistingConversationID_Continues verifies that a valid existing
// conversation_id continues the conversation (multi-turn).
func TestUseCase_ExistingConversationID_Continues(t *testing.T) {
	ctx := t.Context()

	existingConv := &domain.ConversationState{
		ID:        "conv-existing-123",
		UserID:    "",
		Messages: []domain.ConversationMessage{
			{Role: "user", Content: "Hola"},
			{Role: "assistant", Content: "¿En qué puedo ayudarte?"},
		},
		TurnCount: 1,
		MaxTurns:  5,
	}

	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{
		convs: map[string]*domain.ConversationState{
			"conv-existing-123": existingConv,
		},
	}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{
		Message:        "Vuelos a Madrid",
		ConversationID: "conv-existing-123",
	}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error for existing conversation, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.ConversationID != "conv-existing-123" {
		t.Errorf("expected same conversation_id, got %q", resp.ConversationID)
	}
	if resp.TurnCount != 2 {
		t.Errorf("expected turn_count 2, got %d", resp.TurnCount)
	}
}

// =============================================================================
// W4 — Provider switch test (R7)
// Both DeepSeek and Ollama adapters must satisfy AIInterpreter.
// =============================================================================

// TestProviderSwitch_BothAdaptersSatisfyInterface verifies that both
// DeepSeek and Ollama adapters satisfy domain.AIInterpreter at compile time.
// The adapters already assert this via `var _ domain.AIInterpreter = (*Adapter)(nil)`
// in their respective packages. This test documents the swappable contract.
func TestProviderSwitch_BothAdaptersSatisfyInterface(t *testing.T) {
	// Both adapters are swappable via AI_SEARCH_PROVIDER env var in module.go.
	// This test validates the contract: any AIInterpreter can be wired into UseCase.
	// The compile-time checks in deepseek/adapter.go and ollama/adapter.go
	// enforce that both adapters satisfy the interface.

	// Verify UseCaseDeps accepts the interface (not a concrete type)
	deps := ai_search.UseCaseDeps{
		AIInterpreter: nil, // nil is valid — handler returns 503
	}
	_ = deps

	// Verify NewUseCase creates a valid usecase with nil interpreter
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  nil,
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      &stubConversationStore{},
	})
	if uc == nil {
		t.Fatal("NewUseCase should return non-nil even with nil interpreter")
	}

	t.Log("AIInterpreter interface swapability validated — both adapters compile with interface check")
}

// =============================================================================
// IATA normalization tests
// =============================================================================

// staticIATAResolver returns a resolver backed by a static map.
// Used in tests to mock IATA code resolution.
func staticIATAResolver(codes map[string]string) func(ctx context.Context, query string) (string, error) {
	return func(_ context.Context, query string) (string, error) {
		if code, ok := codes[query]; ok {
			return code, nil
		}
		return "", nil
	}
}

// TestUseCase_IATAResolution_NormalizesCityNames verifies that when the AI
// returns city names (e.g. "Madrid", "París") as departure/arrival,
// the IATA resolver normalizes them to proper IATA codes ("MAD", "CDG")
// BEFORE calling the flight searcher.
func TestUseCase_IATAResolution_NormalizesCityNames(t *testing.T) {
	ctx := t.Context()

	// AI returns city names — not valid IATA codes
	intent := &domain.TravelIntent{
		Type:       "flights",
		Confidence: 0.90,
		FlightParams: &domain.FlightSearchRequest{
			Departure:    "Madrid",
			Arrival:      "París",
			OutboundDate: "2026-06-15",
			ReturnDate:   "2026-06-30",
			Adults:       1,
			TripType:     "round_trip",
		},
	}

	interpreter := &stubInterpreter{intent: intent}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	// IATA resolver maps city names to codes
	resolver := staticIATAResolver(map[string]string{
		"Madrid": "MAD",
		"París":  "CDG",
	})

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		IATAResolver:   resolver,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Vuelo de Madrid a París"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Verify the searcher received normalized IATA codes, NOT city names
	if flightsSearcher.lastCmd.Departure != "MAD" {
		t.Errorf("expected departure 'MAD' (resolved from 'Madrid'), got %q",
			flightsSearcher.lastCmd.Departure)
	}
	if flightsSearcher.lastCmd.Arrival != "CDG" {
		t.Errorf("expected arrival 'CDG' (resolved from 'París'), got %q",
			flightsSearcher.lastCmd.Arrival)
	}

	t.Logf("IATA resolution: 'Madrid' → %q, 'París' → %q",
		flightsSearcher.lastCmd.Departure, flightsSearcher.lastCmd.Arrival)
}

// TestUseCase_ParamNormalization_NumericToEnums verifies that numeric values
// from the AI (stops=0, sort_by=1, travel_class=1, type=1) are normalized
// to their correct string equivalents.
func TestUseCase_ParamNormalization_NumericToEnums(t *testing.T) {
	ctx := t.Context()

	// AI returns numeric values — invalid for SerpAPI
	intent := &domain.TravelIntent{
		Type:       "flights",
		Confidence: 0.85,
		FlightParams: &domain.FlightSearchRequest{
			Departure:    "MAD", // already valid IATA — should pass through
			Arrival:      "BCN", // already valid IATA — should pass through
			OutboundDate: "2025-06-15", // year should be adjusted to 2026
			ReturnDate:   "2025-07-01", // year should be adjusted to 2026
			Adults:       1,
			Stops:        "0", // should normalize to "nonstop"
			SortBy:       "1", // should normalize to "top"
			TravelClass:  "1", // should normalize to "economy"
			TripType:     "1", // should normalize to "round_trip"
		},
	}

	interpreter := &stubInterpreter{intent: intent}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	// No IATA resolver needed — params are already valid IATA codes
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	cmd := ai_search.Command{Message: "Vuelo directo MAD-BCN"}
	resp, err := uc.Execute(ctx, cmd, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Verify numeric values were normalized
	if flightsSearcher.lastCmd.Stops != "nonstop" {
		t.Errorf("stops: expected 'nonstop' (from '0'), got %q", flightsSearcher.lastCmd.Stops)
	}
	if flightsSearcher.lastCmd.SortBy != "top" {
		t.Errorf("sort_by: expected 'top' (from '1'), got %q", flightsSearcher.lastCmd.SortBy)
	}
	if flightsSearcher.lastCmd.TravelClass != "economy" {
		t.Errorf("travel_class: expected 'economy' (from '1'), got %q", flightsSearcher.lastCmd.TravelClass)
	}
	if flightsSearcher.lastCmd.TripType != "round_trip" {
		t.Errorf("trip_type: expected 'round_trip' (from '1'), got %q", flightsSearcher.lastCmd.TripType)
	}
	// IATA codes should pass through unchanged
	if flightsSearcher.lastCmd.Departure != "MAD" {
		t.Errorf("departure: expected 'MAD' unchanged, got %q", flightsSearcher.lastCmd.Departure)
	}
	// Date year should be adjusted
	if flightsSearcher.lastCmd.OutboundDate[:4] != "2026" {
		t.Errorf("outbound_date: expected year 2026, got %s", flightsSearcher.lastCmd.OutboundDate)
	}
	if flightsSearcher.lastCmd.ReturnDate[:4] != "2026" {
		t.Errorf("return_date: expected year 2026, got %s", flightsSearcher.lastCmd.ReturnDate)
	}
}

// =============================================================================
// Task 2.2-2.4 — Tool call execution, result messages, partial failures
// =============================================================================

func TestExecuteToolCalls_ConcurrentDispatch(t *testing.T) {
	// RED: executeToolCalls method does not exist yet on UseCase
	ctx := t.Context()

	flightSearcher := &stubFlightSearcher{resp: &search_flights.Response{}}
	hotelSearcher := &stubHotelSearcher{resp: &search_hotels.Response{}}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		FlightSearcher: flightSearcher,
		HotelSearcher:  hotelSearcher,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	toolCalls := []ai_search.ToolCall{
		{ID: "call_1", Name: "search_hotels", Arguments: map[string]interface{}{
			"query":          "Barcelona",
			"check_in_date":  "2026-07-01",
			"check_out_date": "2026-07-05",
		}},
		{ID: "call_2", Name: "search_flights", Arguments: map[string]interface{}{
			"trip_type":     "round_trip",
			"departure":     "MAD",
			"arrival":       "BCN",
			"outbound_date": "2026-07-01",
			"return_date":   "2026-07-05",
		}},
	}

	results := uc.ExecuteToolCalls(ctx, toolCalls)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !flightSearcher.called {
		t.Error("flight searcher should have been called")
	}
	if !hotelSearcher.called {
		t.Error("hotel searcher should have been called")
	}
}

func TestBuildToolResultMessages(t *testing.T) {
	// RED: BuildToolResultMessages does not exist yet
	results := []ai_search.ToolResult{
		{
			CallID:      "call_1",
			Name:        "search_hotels",
			Destination: "Barcelona",
			Content:     `{"properties": []}`,
		},
	}

	messages := ai_search.BuildToolResultMessages(results)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg := messages[0]
	if msg.Role != "tool" {
		t.Errorf("Role = %q, want 'tool'", msg.Role)
	}
	if msg.ToolCallID != "call_1" {
		t.Errorf("ToolCallID = %q, want 'call_1'", msg.ToolCallID)
	}
	if msg.Content != `{"properties": []}` {
		t.Errorf("Content = %q, want the result JSON", msg.Content)
	}
}

func TestBuildToolResultMessages_WithError(t *testing.T) {
	results := []ai_search.ToolResult{
		{
			CallID: "call_err",
			Name:   "search_flights",
			Error:  errors.New("provider timeout"),
		},
	}

	messages := ai_search.BuildToolResultMessages(results)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg := messages[0]

	// Error should be in the content as JSON
	if !strings.Contains(msg.Content, "provider timeout") {
		t.Errorf("Content should contain error, got: %s", msg.Content)
	}
}

func TestExecuteToolCalls_PartialFailure(t *testing.T) {
	// One tool fails, other succeeds — both results collected
	ctx := t.Context()

	hotelSearcher := &stubHotelSearcher{
		err:  errors.New("search_hotels unavailable"),
		resp: &search_hotels.Response{},
	}
	flightSearcher := &stubFlightSearcher{
		resp: &search_flights.Response{BestFlights: []domain.Flight{{
			Legs: []domain.Leg{{Airline: "IB"}},
		}}},
	}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		FlightSearcher: flightSearcher,
		HotelSearcher:  hotelSearcher,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	toolCalls := []ai_search.ToolCall{
		{ID: "call_h", Name: "search_hotels", Arguments: map[string]interface{}{
			"query": "Barcelona", "check_in_date": "2026-07-01", "check_out_date": "2026-07-05",
		}},
		{ID: "call_f", Name: "search_flights", Arguments: map[string]interface{}{
			"trip_type": "round_trip", "departure": "MAD", "arrival": "BCN",
			"outbound_date": "2026-07-01", "return_date": "2026-07-05",
		}},
	}

	results := uc.ExecuteToolCalls(ctx, toolCalls)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result should have error (hotel)
	if results[0].CallID != "call_h" {
		// The order depends on wg.Go() scheduling, but both should be present
	}

	// Count errors
	errCount := 0
	okCount := 0
	for _, r := range results {
		if r.Error != nil {
			errCount++
		} else {
			okCount++
		}
	}
	if errCount != 1 {
		t.Errorf("expected 1 error result, got %d", errCount)
	}
	if okCount != 1 {
		t.Errorf("expected 1 success result, got %d", okCount)
	}

	// Build messages from partial results — should produce valid tool messages
	messages := ai_search.BuildToolResultMessages(results)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	// Error message should contain error info
	foundError := false
	for _, m := range messages {
		if strings.Contains(m.Content, "search_hotels unavailable") {
			foundError = true
		}
	}
	if !foundError {
		t.Error("expected error content in at least one tool result message")
	}
}

// =============================================================================
// Task 3.2-3.3 — Streaming orchestration + multi-turn
// =============================================================================

// stubToolCallStreamer implements domain.ToolCallStreamer for testing.
type stubToolCallStreamer struct {
	calls    int
	responses []*domain.ToolCallStreamResult
}

func (s *stubToolCallStreamer) ChatWithTools(ctx context.Context, messages []domain.ChatMessage, tools []map[string]interface{}) (*domain.ToolCallStreamResult, error) {
	if s.calls < len(s.responses) {
		resp := s.responses[s.calls]
		s.calls++
		return resp, nil
	}
	return &domain.ToolCallStreamResult{AssistantText: "done"}, nil
}

func TestExecuteChatStream_SingleToolCall(t *testing.T) {
	// RED: ExecuteChatStream does not exist on UseCase yet
	ctx := t.Context()
	w := httptest.NewRecorder()

	flightSearcher := &stubFlightSearcher{resp: &search_flights.Response{}}

	streamer := &stubToolCallStreamer{
		responses: []*domain.ToolCallStreamResult{
			{
				AssistantText: "Voy a buscar vuelos.",
				ToolCalls: []domain.ToolCall{
					{
						ID:   "call_1",
						Name: "search_flights",
						Arguments: map[string]interface{}{
							"trip_type":     "round_trip",
							"departure":     "MAD",
							"arrival":       "BCN",
							"outbound_date": "2026-07-01",
							"return_date":   "2026-07-05",
						},
					},
				},
			},
			{
				// Second AI response (after results injected)
				AssistantText: "Encontré vuelos de Madrid a Barcelona.",
			},
		},
	}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		FlightSearcher:         flightSearcher,
		HotelSearcher:          &stubHotelSearcher{resp: &search_hotels.Response{}},
		ToolCallStreamer:       streamer,
		AnonMaxTurns:           5,
		AuthMaxTurns:           10,
	})

	messages := []domain.ChatMessage{
		{Role: "system", Content: "Eres un asistente de viajes."},
		{Role: "user", Content: "busco vuelos Madrid-Barcelona"},
	}
	tools := []map[string]interface{}{
		{"type": "function", "function": map[string]interface{}{
			"name": "search_flights",
			"parameters": map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{},
			},
		}},
	}

	turnCount, err := uc.ExecuteChatStream(ctx, w, messages, tools, 3)
	if err != nil {
		t.Fatalf("ExecuteChatStream failed: %v", err)
	}

	if turnCount != 1 {
		t.Errorf("turnCount = %d, want 1", turnCount)
	}

	body := w.Body.String()

	// Should emit: chunk* → search → chunk* → done
	hasChunk := strings.Contains(body, "event: chunk")
	hasSearch := strings.Contains(body, "event: search")
	hasDone := strings.Contains(body, "event: done")

	if !hasChunk {
		t.Error("SSE stream should contain chunk events")
	}
	if !hasSearch {
		t.Error("SSE stream should contain search event")
	}
	if !hasDone {
		t.Error("SSE stream should contain done event")
	}
}

func TestExecuteChatStream_MultiTurn(t *testing.T) {
	// Two rounds of tool calls
	ctx := t.Context()
	w := httptest.NewRecorder()

	hotelSearcher := &stubHotelSearcher{resp: &search_hotels.Response{}}
	flightSearcher := &stubFlightSearcher{resp: &search_flights.Response{}}

	streamer := &stubToolCallStreamer{
		responses: []*domain.ToolCallStreamResult{
			{
				AssistantText: "Busco hoteles...",
				ToolCalls: []domain.ToolCall{
					{ID: "call_h", Name: "search_hotels", Arguments: map[string]interface{}{
						"query": "Barcelona", "check_in_date": "2026-07-01", "check_out_date": "2026-07-05",
					}},
				},
			},
			{
				// After hotel results: AI decides to search flights too
				AssistantText: "Y también busco vuelos...",
				ToolCalls: []domain.ToolCall{
					{ID: "call_f", Name: "search_flights", Arguments: map[string]interface{}{
						"trip_type": "round_trip", "departure": "MAD", "arrival": "BCN",
						"outbound_date": "2026-07-01", "return_date": "2026-07-05",
					}},
				},
			},
			{
				AssistantText: "Resultados completos.",
			},
		},
	}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		FlightSearcher:   flightSearcher,
		HotelSearcher:    hotelSearcher,
		ToolCallStreamer: streamer,
		AnonMaxTurns:     5,
		AuthMaxTurns:     10,
	})

	messages := []domain.ChatMessage{
		{Role: "user", Content: "viaje a Barcelona"},
	}
	tools := []map[string]interface{}{}

	turnCount, err := uc.ExecuteChatStream(ctx, w, messages, tools, 3)
	if err != nil {
		t.Fatalf("ExecuteChatStream failed: %v", err)
	}

	if turnCount != 2 {
		t.Errorf("turnCount = %d, want 2 (two tool call rounds)", turnCount)
	}

	body := w.Body.String()
	// Should have 2 search events
	searchCount := strings.Count(body, "event: search")
	if searchCount != 2 {
		t.Errorf("expected 2 search events, got %d", searchCount)
	}
}

func TestExecuteChatStream_MaxTurnsGuard(t *testing.T) {
	// AI keeps requesting tool calls — should stop at maxTurns
	ctx := t.Context()
	w := httptest.NewRecorder()

	streamer := &stubToolCallStreamer{
		responses: []*domain.ToolCallStreamResult{
			{AssistantText: "round 1", ToolCalls: []domain.ToolCall{
				{ID: "c1", Name: "search_hotels", Arguments: map[string]interface{}{
					"query": "X", "check_in_date": "2026-01-01", "check_out_date": "2026-01-02",
				}},
			}},
			{AssistantText: "round 2", ToolCalls: []domain.ToolCall{
				{ID: "c2", Name: "search_hotels", Arguments: map[string]interface{}{
					"query": "Y", "check_in_date": "2026-01-01", "check_out_date": "2026-01-02",
				}},
			}},
			{AssistantText: "final"},
		},
	}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		FlightSearcher:   &stubFlightSearcher{resp: &search_flights.Response{}},
		HotelSearcher:    &stubHotelSearcher{resp: &search_hotels.Response{}},
		ToolCallStreamer: streamer,
		AnonMaxTurns:     5,
		AuthMaxTurns:     10,
	})

	messages := []domain.ChatMessage{{Role: "user", Content: "x"}}
	tools := []map[string]interface{}{}

	// maxTurns = 1 — should stop after first tool call round
	turnCount, err := uc.ExecuteChatStream(ctx, w, messages, tools, 1)
	if err != nil {
		t.Fatalf("ExecuteChatStream failed: %v", err)
	}

	if turnCount != 1 {
		t.Errorf("turnCount = %d, want 1 (capped by maxTurns)", turnCount)
	}

	body := w.Body.String()
	searchCount := strings.Count(body, "event: search")
	if searchCount > 1 {
		t.Errorf("expected at most 1 search event with maxTurns=1, got %d", searchCount)
	}
}
