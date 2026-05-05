// Integration tests for the ai_search handler: end-to-end behavior.
// Verifies full conversation flow, search orchestration, turn limits,
// and error handling through the HTTP handler layer.
//
// Tests the handler → usecase → stubbed dependencies pipeline
// with stateful conversation persistence across turns.
//
// DEPENDENCIES: relies on stub types from usecase_test.go:
//   stubFlightSearcher, stubHotelSearcher, stubConversationStore
package ai_search_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// integrationDefaults is the fallback config for handler integration tests.
var integrationDefaults = shared.SearchDefaultConfig{
	Currency:    "EUR",
	Language:    "es",
	CountryCode: "AR",
}

// =============================================================================
// Test init: register domain error mappers (normally done by module bootstrap)
// =============================================================================

func init() {
	serrors.RegisterDomainErrorMapper(func(err error) *serrors.Problem {
		switch {
		case errors.Is(err, domain.ErrAIUnavailable):
			return serrors.ErrServiceUnavailable("Servicio de IA no disponible", err)
		case errors.Is(err, domain.ErrAIParseFailure):
			return serrors.ErrBadGateway("La IA devolvió una respuesta inválida", err)
		case errors.Is(err, domain.ErrTurnLimitExceeded):
			return serrors.ErrBadRequest("Se alcanzó el límite máximo de turnos en la conversación", err)
		case errors.Is(err, domain.ErrConversationNotFound):
			return serrors.ErrBadRequest("Conversación no encontrada o expirada", err)
		}
		return nil
	})
}

// =============================================================================
// Integration-specific stub: multi-turn AI interpreter
// =============================================================================

// multiTurnInterpreter returns different intents per call for multi-turn simulation.
type multiTurnInterpreter struct {
	intents   []*domain.TravelIntent
	errors    []error
	callIdx   int
	parseCalls []parseCall
}

type parseCall struct {
	message    string
	historyLen int
}

func (s *multiTurnInterpreter) Parse(ctx context.Context, message string, history []domain.ConversationMessage, language string) (*domain.TravelIntent, error) {
	s.parseCalls = append(s.parseCalls, parseCall{message: message, historyLen: len(history)})

	idx := s.callIdx
	s.callIdx++

	if idx < len(s.errors) && s.errors[idx] != nil {
		return nil, s.errors[idx]
	}
	if idx < len(s.intents) {
		return s.intents[idx], nil
	}
	return nil, errors.New("no intent configured for call index")
}

// =============================================================================
// Integration-specific stub: stateful conversation store
// =============================================================================

// statefulConvStore persists conversations in memory and saves with TTL semantics.
// Has saveCalls counter for verification.
type statefulConvStore struct {
	convs     map[string]*domain.ConversationState
	saveCalls int
}

func (s *statefulConvStore) GetConversation(ctx context.Context, id string) (*domain.ConversationState, error) {
	if s.convs == nil {
		return nil, nil
	}
	return s.convs[id], nil
}

func (s *statefulConvStore) SaveConversation(ctx context.Context, conv *domain.ConversationState) error {
	if s.convs == nil {
		s.convs = make(map[string]*domain.ConversationState)
	}
	s.convs[conv.ID] = conv
	s.saveCalls++
	return nil
}

// =============================================================================
// Integration-specific stub: AI interpreter that returns a wrapped ErrAIParseFailure
// =============================================================================

// parseFailureInterpreter always returns domain.ErrAIParseFailure.
type parseFailureInterpreter struct{}

func (p *parseFailureInterpreter) Parse(ctx context.Context, message string, history []domain.ConversationMessage, language string) (*domain.TravelIntent, error) {
	return nil, domain.ErrAIParseFailure
}

// =============================================================================
// Test helpers
// =============================================================================

func newAIEchoContext(body string) (*echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/v1/search/ai", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	return c, rec
}

func mustParseMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal response body: %v", err)
	}
	return raw
}

// =============================================================================
// 7.1 — Full conversation flow test
// =============================================================================

// TestIntegration_FullConversationFlow simulates a multi-turn conversation:
//   Turn 1: incomplete message → follow-up question
//   Turn 2: follow-up with destination → still incomplete
//   Turn 3: complete query → flights search triggered → results returned
// Verifies conversation state is persisted across turns.
func TestIntegration_FullConversationFlow(t *testing.T) {
	interpreter := &multiTurnInterpreter{
		intents: []*domain.TravelIntent{
			newIncompleteIntent(), // Turn 1: "I want to travel" → missing fields
			newIncompleteIntent(), // Turn 2: "To Barcelona" → still missing dates
			newFlightsIntent(),    // Turn 3: complete → triggers flights search
		},
	}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &statefulConvStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	// ---- Turn 1: Incomplete message ----
	c1, rec1 := newAIEchoContext(`{"message": "Quiero viajar"}`)

	err1 := handler.Handle(c1)
	if err1 != nil {
		t.Fatalf("Turn 1: expected no error, got %v", err1)
	}
	if rec1.Code != http.StatusOK {
		t.Fatalf("Turn 1: expected 200, got %d", rec1.Code)
	}

	resp1 := mustParseMap(t, rec1.Body.Bytes())
	convID, ok := resp1["conversation_id"].(string)
	if !ok || convID == "" {
		t.Fatal("Turn 1: expected non-empty conversation_id")
	}

	// Verify response shape
	if got, ok := resp1["intent"].(string); !ok || got != "incomplete" {
		t.Errorf("Turn 1: expected intent 'incomplete', got %v", resp1["intent"])
	}
	if msg, ok := resp1["message"].(string); !ok || msg == "" {
		t.Error("Turn 1: expected follow-up message, got empty")
	}
	if tc, ok := resp1["turn_count"].(float64); !ok || tc != 1 {
		t.Errorf("Turn 1: expected turn_count 1, got %v", resp1["turn_count"])
	}
	if flightsSearcher.called {
		t.Error("Turn 1: flights search should NOT be called for incomplete intent")
	}

	// ---- Turn 2: Follow-up with destination (still incomplete) ----
	flightsSearcher.called = false // reset

	c2, rec2 := newAIEchoContext(`{"message": "A Barcelona", "conversation_id": "` + convID + `"}`)

	err2 := handler.Handle(c2)
	if err2 != nil {
		t.Fatalf("Turn 2: expected no error, got %v", err2)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("Turn 2: expected 200, got %d", rec2.Code)
	}

	resp2 := mustParseMap(t, rec2.Body.Bytes())
	if got, ok := resp2["conversation_id"].(string); !ok || got != convID {
		t.Errorf("Turn 2: expected same conversation_id %q, got %v", convID, resp2["conversation_id"])
	}
	if tc, ok := resp2["turn_count"].(float64); !ok || tc != 2 {
		t.Errorf("Turn 2: expected turn_count 2, got %v", resp2["turn_count"])
	}

	// Verify conversation history was passed to interpreter (should have 2+ messages from Turn 1)
	foundHistory := false
	for _, pc := range interpreter.parseCalls {
		if pc.historyLen >= 2 && pc.message == "A Barcelona" {
			foundHistory = true
			break
		}
	}
	if !foundHistory {
		t.Errorf("Turn 2: expected interpreter to receive conversation history (>=2 msgs), got: %v", interpreter.parseCalls)
	}

	// ---- Turn 3: Complete query — triggers search ----
	flightsSearcher.called = false // reset
	// Set up flights searcher to return non-trivial results
	flightsSearcher.resp = &search_flights.Response{
		BestFlights: []domain.Flight{{}},
		OtherFlights: []domain.Flight{{}},
	}

	c3, rec3 := newAIEchoContext(`{"message": "Vuelos de Buenos Aires a Madrid el 15 de junio", "conversation_id": "` + convID + `"}`)

	err3 := handler.Handle(c3)
	if err3 != nil {
		t.Fatalf("Turn 3: expected no error, got %v", err3)
	}
	if rec3.Code != http.StatusOK {
		t.Fatalf("Turn 3: expected 200, got %d", rec3.Code)
	}

	resp3 := mustParseMap(t, rec3.Body.Bytes())
	if got, ok := resp3["intent"].(string); !ok || got != "flights" {
		t.Errorf("Turn 3: expected intent 'flights', got %v", resp3["intent"])
	}
	if !flightsSearcher.called {
		t.Error("Turn 3: flights search should be called for flights intent")
	}
	if resp3["flights"] == nil {
		t.Error("Turn 3: expected flights in response")
	}
	if tc, ok := resp3["turn_count"].(float64); !ok || tc != 3 {
		t.Errorf("Turn 3: expected turn_count 3, got %v", resp3["turn_count"])
	}

	// Verify full conversation history propagated to Turn 3 interpreter (4+ messages)
	foundHistory3 := false
	for _, pc := range interpreter.parseCalls {
		if pc.message == "Vuelos de Buenos Aires a Madrid el 15 de junio" && pc.historyLen >= 4 {
			foundHistory3 = true
			break
		}
	}
	if !foundHistory3 {
		t.Errorf("Turn 3: expected interpreter to receive full conversation history (>=4 msgs), got: %v", interpreter.parseCalls)
	}
}

// =============================================================================
// 7.2 — Search orchestration test
// =============================================================================

// TestIntegration_SearchOrchestration_Flights verifies that a "flights" intent
// triggers ONLY the flights use case, not hotels.
func TestIntegration_SearchOrchestration_Flights(t *testing.T) {
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Vuelos de Buenos Aires a Madrid"}`)
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	resp := mustParseMap(t, rec.Body.Bytes())
	if got, ok := resp["intent"].(string); !ok || got != "flights" {
		t.Errorf("expected intent 'flights', got %v", resp["intent"])
	}
	if !flightsSearcher.called {
		t.Error("flights searcher should be called")
	}
	if hotelsSearcher.called {
		t.Error("hotels searcher should NOT be called for flights intent")
	}
}

// TestIntegration_SearchOrchestration_Hotels verifies that a "hotels" intent
// triggers ONLY the hotels use case, not flights.
func TestIntegration_SearchOrchestration_Hotels(t *testing.T) {
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Hoteles en Barcelona"}`)
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	resp := mustParseMap(t, rec.Body.Bytes())
	if got, ok := resp["intent"].(string); !ok || got != "hotels" {
		t.Errorf("expected intent 'hotels', got %v", resp["intent"])
	}
	if !hotelsSearcher.called {
		t.Error("hotels searcher should be called")
	}
	if flightsSearcher.called {
		t.Error("flights searcher should NOT be called for hotels intent")
	}
}

// TestIntegration_SearchOrchestration_Both verifies that a "both" intent
// triggers BOTH the flights and hotels use cases.
func TestIntegration_SearchOrchestration_Both(t *testing.T) {
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Viaje completo a Barcelona con vuelo y hotel"}`)
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	resp := mustParseMap(t, rec.Body.Bytes())
	if got, ok := resp["intent"].(string); !ok || got != "both" {
		t.Errorf("expected intent 'both', got %v", resp["intent"])
	}
	if !flightsSearcher.called {
		t.Error("flights searcher should be called for both intent")
	}
	if !hotelsSearcher.called {
		t.Error("hotels searcher should be called for both intent")
	}
}

// =============================================================================
// 7.3 — Turn limit test
// =============================================================================

// TestIntegration_TurnLimit_Anonymous verifies that anonymous users cannot
// exceed 5 turns. The 6th turn returns 400 Bad Request.
func TestIntegration_TurnLimit_Anonymous(t *testing.T) {
	interpreter := &stubInterpreter{intent: newIncompleteIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	// Pre-populate a conversation with 5 turns (anon max)
	convStore.convs = map[string]*domain.ConversationState{
		"conv_anon_full": {
			ID:        "conv_anon_full",
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Quiero más opciones", "conversation_id": "conv_anon_full"}`)

	// handler.Handle() returns nil — errors are mapped to JSON responses
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected nil (MapError writes JSON), got: %v", err)
	}

	// Verify 400 Bad Request
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	// Verify response body contains turn limit info
	resp := mustParseMap(t, rec.Body.Bytes())
	if detail, ok := resp["detail"].(string); ok {
		if detail == "" {
			t.Error("expected non-empty detail in error response")
		}
	}

	// Verify no search was attempted
	if flightsSearcher.called {
		t.Error("flights search should NOT be called when turn limit exceeded")
	}
	if hotelsSearcher.called {
		t.Error("hotels search should NOT be called when turn limit exceeded")
	}
}

// TestIntegration_TurnLimit_Authenticated verifies that authenticated users
// have 10 turns max. The 11th turn returns 400 Bad Request.
func TestIntegration_TurnLimit_Authenticated(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	interpreter := &stubInterpreter{intent: newIncompleteIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}

	// Pre-populate a conversation with 10 turns (auth max)
	convStore.convs = map[string]*domain.ConversationState{
		"conv_auth_full": {
			ID:        "conv_auth_full",
			UserID:    userID.String(),
			TurnCount: 10,
			MaxTurns:  10,
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Una consulta más", "conversation_id": "conv_auth_full"}`)

	// Set auth claims
	c.Set("user_claims", &token.AccessClaims{
		UserID:    userID,
		Email:     "test@example.com",
		RoleID:    uuid.Nil,
		SessionID: uuid.Nil,
		JTI:       uuid.Nil,
	})

	// handler.Handle() returns nil — errors are mapped to JSON responses
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected nil (MapError writes JSON), got: %v", err)
	}

	// Verify 400 Bad Request
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	// Verify no search was attempted
	if flightsSearcher.called {
		t.Error("flights search should NOT be called when turn limit exceeded")
	}
}

// =============================================================================
// 7.4 — Error handling test
// =============================================================================

// TestIntegration_ErrorHandling_AIUnavailable verifies that when the AI
// interpreter returns an error, the handler returns 503 Service Unavailable.
func TestIntegration_ErrorHandling_AIUnavailable(t *testing.T) {
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Vuelos baratos"}`)

	// handler.Handle() returns nil — errors are mapped to JSON responses
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected nil (MapError writes JSON), got: %v", err)
	}

	// Verify 503 Service Unavailable
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}

	// Verify response body contains service unavailable info
	resp := mustParseMap(t, rec.Body.Bytes())
	if status, ok := resp["status"].(float64); !ok || int(status) != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %v", resp["status"])
	}
}

// TestIntegration_ErrorHandling_AIParseFailure verifies that when the AI
// interpreter returns a parse failure, the handler returns 502 Bad Gateway.
func TestIntegration_ErrorHandling_AIParseFailure(t *testing.T) {
	interpreter := &parseFailureInterpreter{}
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "malformed query !@#$%"}`)

	// handler.Handle() returns nil — errors are mapped to JSON responses
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected nil (MapError writes JSON), got: %v", err)
	}

	// Verify 502 Bad Gateway
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rec.Code)
	}

	// Verify response body indicates the error type
	resp := mustParseMap(t, rec.Body.Bytes())
	if status, ok := resp["status"].(float64); !ok || int(status) != http.StatusBadGateway {
		t.Errorf("expected status 502, got %v", resp["status"])
	}
}

// TestIntegration_ErrorHandling_InvalidConversationID verifies that a non-existent
// or expired conversation ID returns 400 Bad Request (NOT silently creates a new one).
func TestIntegration_ErrorHandling_InvalidConversationID(t *testing.T) {
	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{}
	// No pre-populated conversations — any ID will not be found

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Vuelos a Madrid", "conversation_id": "nonexistent-conv-999"}`)

	// handler.Handle() returns nil — errors are mapped to JSON responses
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected nil (MapError writes JSON), got: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for non-existent conversation, got %d", rec.Code)
	}

	// Verify response body contains the error detail
	resp := mustParseMap(t, rec.Body.Bytes())
	if detail, ok := resp["detail"].(string); !ok || detail == "" {
		t.Error("expected non-empty detail in error response")
	}
	if status, ok := resp["status"].(float64); !ok || int(status) != http.StatusBadRequest {
		t.Errorf("expected status 400, got %v", resp["status"])
	}
}

// TestIntegration_ErrorHandling_EmptyMessage verifies that an empty message
// returns 400 Bad Request.
func TestIntegration_ErrorHandling_EmptyMessage(t *testing.T) {
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  &stubInterpreter{},
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      &stubConversationStore{},
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": ""}`)

	err := handler.Handle(c)
	if err == nil {
		t.Fatal("expected error for empty message")
	}

	var he *echo.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected HTTPError, got: %v", err)
	}
	if he.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", he.Code)
	}
	_ = rec
}

// =============================================================================
// 7.5 — Nil usecase → 503 test
// =============================================================================

// TestIntegration_NilUseCase_Returns503 verifies that when the handler has
// a nil usecase (AI interpreter not configured at bootstrap), the handler
// returns 503 Service Unavailable with RFC 9457 format instead of 404.
func TestIntegration_NilUseCase_Returns503(t *testing.T) {
	// Create handler with nil usecase (simulates AI not configured)
	handler := ai_search.NewHandler(nil, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Busco vuelos a Madrid"}`)

	// handler.Handle() returns nil — error is written via c.JSON
	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected nil (c.JSON writes response directly), got: %v", err)
	}

	// Verify 503 Service Unavailable (NOT 404)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}

	// Verify RFC 9457 Problem format
	resp := mustParseMap(t, rec.Body.Bytes())
	if status, ok := resp["status"].(float64); !ok || int(status) != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %v", resp["status"])
	}
	if title, ok := resp["title"].(string); !ok || title == "" {
		t.Error("expected non-empty title in 503 response")
	}
	if detail, ok := resp["detail"].(string); !ok || detail == "" {
		t.Error("expected non-empty detail in 503 response")
	}
	if typ, ok := resp["type"].(string); !ok || typ == "" {
		t.Error("expected non-empty type in 503 response")
	}
}

// =============================================================================
// Conversation ID validation tests — Fix 2 and Fix 3
// =============================================================================

// TestIntegration_NewConversation_EmptyConversationID verifies that an empty
// conversation_id (first request) creates a new conversation with 200 OK.
func TestIntegration_NewConversation_EmptyConversationID(t *testing.T) {
	interpreter := &stubInterpreter{intent: newAmbiguousIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &statefulConvStore{}

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	// No conversation_id at all — first request
	c, rec := newAIEchoContext(`{"message": "Quiero viajar a Barcelona"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected no error for new conversation, got: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	resp := mustParseMap(t, rec.Body.Bytes())
	convID, ok := resp["conversation_id"].(string)
	if !ok || convID == "" {
		t.Fatal("expected non-empty conversation_id in response")
	}
	if tc, ok := resp["turn_count"].(float64); !ok || int(tc) != 1 {
		t.Errorf("expected turn_count 1, got %v", resp["turn_count"])
	}
}

// TestIntegration_NewConversation_RejectsProvidedConversationID verifies that
// providing a non-existent conversation_id on what looks like a first request
// returns 400 Bad Request (not silently creating a new conversation).
func TestIntegration_NewConversation_RejectsProvidedConversationID(t *testing.T) {
	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &stubConversationStore{} // empty — no conversations stored

	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  interpreter,
		FlightSearcher: flightsSearcher,
		HotelSearcher:  hotelsSearcher,
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	// User provides a conversation_id that doesn't exist
	c, rec := newAIEchoContext(`{"message": "Busco vuelos a Madrid", "conversation_id": "conv-nonexistent"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected nil (MapError writes JSON), got: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-existent conversation_id, got %d", rec.Code)
	}

	resp := mustParseMap(t, rec.Body.Bytes())
	if status, ok := resp["status"].(float64); !ok || int(status) != http.StatusBadRequest {
		t.Errorf("expected status 400, got %v", resp["status"])
	}
	if detail, ok := resp["detail"].(string); !ok || detail == "" {
		t.Error("expected non-empty detail in 400 response")
	}
}

// TestIntegration_ContinueConversation_ValidConversationID verifies that
// providing a valid existing conversation_id continues the conversation
// (multi-turn / multi-device scenario).
func TestIntegration_ContinueConversation_ValidConversationID(t *testing.T) {
	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	convStore := &statefulConvStore{
		convs: map[string]*domain.ConversationState{
			"conv-valid-123": {
				ID:        "conv-valid-123",
				UserID:    "",
				Messages: []domain.ConversationMessage{
					{Role: "user", Content: "Hola"},
					{Role: "assistant", Content: "¿A dónde querés viajar?"},
				},
				TurnCount: 1,
				MaxTurns:  5,
			},
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	// Continue existing conversation
	c, rec := newAIEchoContext(`{"message": "Vuelos a Madrid", "conversation_id": "conv-valid-123"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected no error for valid conversation, got: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	resp := mustParseMap(t, rec.Body.Bytes())
	if got, ok := resp["conversation_id"].(string); !ok || got != "conv-valid-123" {
		t.Errorf("expected same conversation_id 'conv-valid-123', got %v", resp["conversation_id"])
	}
	if tc, ok := resp["turn_count"].(float64); !ok || int(tc) != 2 {
		t.Errorf("expected turn_count 2 (continued), got %v", resp["turn_count"])
	}
}

// TestIntegration_NonExistentConversationID_Returns400 verifies that a
// conversation_id that was never created returns 400 Bad Request with
// a clear error message.
func TestIntegration_NonExistentConversationID_Returns400(t *testing.T) {
	interpreter := &stubInterpreter{intent: newFlightsIntent()}
	flightsSearcher := &stubFlightSearcher{}
	hotelsSearcher := &stubHotelSearcher{}
	// Stateful store with a DIFFERENT conversation — ensures the requested one is missing
	convStore := &statefulConvStore{
		convs: map[string]*domain.ConversationState{
			"other-conv": {
				ID:    "other-conv",
				Messages: []domain.ConversationMessage{
					{Role: "user", Content: "Hola"},
				},
				TurnCount: 1,
				MaxTurns:  5,
			},
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

	handler := ai_search.NewHandler(uc, nil, integrationDefaults)

	c, rec := newAIEchoContext(`{"message": "Busco hoteles", "conversation_id": "conv-deleted-or-expired"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("expected nil (MapError writes JSON), got: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-existent conversation_id, got %d", rec.Code)
	}

	resp := mustParseMap(t, rec.Body.Bytes())
	if status, ok := resp["status"].(float64); !ok || int(status) != http.StatusBadRequest {
		t.Errorf("expected status 400, got %v", resp["status"])
	}
}
