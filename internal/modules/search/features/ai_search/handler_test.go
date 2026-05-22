package ai_search_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler test helpers
// =============================================================================

func newEchoContext(body string) (*echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/v1/search/ai", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	return c, rec
}

// defaultCfg is used in handler tests — ResolveSearchDefaults needs a fallback.
var defaultCfg = shared.SearchDefaultConfig{
	Currency: "EUR",
	Language: "es",
}

// =============================================================================
// Handler tests
// =============================================================================

// TestHandler_ValidMessage verifies that a POST with a valid message returns 200.
func TestHandler_ValidMessage(t *testing.T) {
	convStore := &stubConversationStore{}
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  &stubInterpreter{intent: newFlightsIntent()},
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	handler := ai_search.NewHandler(uc, convStore, nil, defaultCfg, nil)

	c, rec := newEchoContext(`{"message": "Busco vuelos de Buenos Aires a Madrid"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify response has expected fields
	var resp ai_search.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.ConversationID == "" {
		t.Error("ConversationID should not be empty")
	}
	if resp.Intent == "" {
		t.Error("Intent should not be empty")
	}
	if resp.Intent != "flights" {
		t.Errorf("Intent = %q, want %q", resp.Intent, "flights")
	}
}

// TestHandler_EmptyMessage verifies that an empty message returns 400.
func TestHandler_EmptyMessage(t *testing.T) {
	convStore := &stubConversationStore{}
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  &stubInterpreter{},
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      convStore,
	})

	handler := ai_search.NewHandler(uc, convStore, nil, defaultCfg, nil)

	c, rec := newEchoContext(`{"message": ""}`)

	err := handler.Handle(c)
	// Handler validates command before usecase and returns HTTPError(400).
	// In direct handler tests (not routed through Echo middleware),
	// the error is returned directly and rec.Code is not set.
	if err == nil {
		t.Fatal("expected error for empty message, got nil")
	}
	he, ok := errors.AsType[*echo.HTTPError](err)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	if he.Code != http.StatusBadRequest {
		t.Errorf("HTTPError code = %d, want %d", he.Code, http.StatusBadRequest)
	}
	_ = rec
}

// TestHandler_AuthUser verifies that an authenticated user's request
// extracts the user ID from auth claims and passes it to the use case.
func TestHandler_AuthUser(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	convStore := &stubConversationStore{}
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  &stubInterpreter{intent: newFlightsIntent()},
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      convStore,
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	handler := ai_search.NewHandler(uc, convStore, nil, defaultCfg, nil)

	c, rec := newEchoContext(`{"message": "Vuelos baratos"}`)

	// Simulate auth middleware: set user claims in Echo context
	c.Set("user_claims", &sharedauth.AccessClaims{
		UserID:    userID,
		Email:     "test@example.com",
		RoleID:    uuid.Nil,
		JTI:       uuid.Nil,
	})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify the conversation was saved with the authenticated user ID
	found := false
	for _, conv := range convStore.convs {
		if conv.UserID == userID.String() {
			found = true
			break
		}
	}
	if !found {
		t.Error("conversation should be saved with authenticated user_id")
	}

	// Verify response includes conversation ID
	var resp ai_search.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.ConversationID == "" {
		t.Error("ConversationID should not be empty")
	}
}

// TestHandler_InvalidJSON verifies that malformed JSON returns 400.
func TestHandler_InvalidJSON(t *testing.T) {
	convStore := &stubConversationStore{}
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  &stubInterpreter{},
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      convStore,
	})

	handler := ai_search.NewHandler(uc, convStore, nil, defaultCfg, nil)

	c, rec := newEchoContext(`not json`)

	// Handler returns HTTPError(400) on bind failure.
	// In direct handler tests, the error is returned directly.
	err := handler.Handle(c)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	he, ok := errors.AsType[*echo.HTTPError](err)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	if he.Code != http.StatusBadRequest {
		t.Errorf("HTTPError code = %d, want %d", he.Code, http.StatusBadRequest)
	}
	_ = rec
}
