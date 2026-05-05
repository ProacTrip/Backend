package ai_search_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
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
	Currency:    "EUR",
	Language:    "es",
	CountryCode: "AR",
}

// =============================================================================
// Handler tests
// =============================================================================

// TestHandler_ValidMessage verifies that a POST with a valid message returns 200.
func TestHandler_ValidMessage(t *testing.T) {
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  &stubInterpreter{intent: newFlightsIntent()},
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      &stubConversationStore{},
		AnonMaxTurns:   5,
		AuthMaxTurns:   10,
	})

	handler := ai_search.NewHandler(uc, nil, defaultCfg)

	c, rec := newEchoContext(`{"message": "Busco vuelos de Buenos Aires a Madrid"}`)

	err := handler.Handle(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify response has expected fields
	var resp ai_search.Response
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.ConversationID)
	assert.NotEmpty(t, resp.Intent)
	assert.Equal(t, "flights", resp.Intent)
}

// TestHandler_EmptyMessage verifies that an empty message returns 400.
// The handler returns a 400 via MapError (without domain error mapping,
// it falls back to generic 500 via the global handler, but the handler
// test validates the handler correctly delegates to the usecase).
func TestHandler_EmptyMessage(t *testing.T) {
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  &stubInterpreter{},
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      &stubConversationStore{},
	})

	handler := ai_search.NewHandler(uc, nil, defaultCfg)

	c, rec := newEchoContext(`{"message": ""}`)

	err := handler.Handle(c)
	// Handler validates command before usecase and returns HTTPError(400).
	// In direct handler tests (not routed through Echo middleware),
	// the error is returned directly and rec.Code is not set.
	assert.Error(t, err)
	var he *echo.HTTPError
	if assert.ErrorAs(t, err, &he) {
		assert.Equal(t, http.StatusBadRequest, he.Code)
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

	handler := ai_search.NewHandler(uc, nil, defaultCfg)

	c, rec := newEchoContext(`{"message": "Vuelos baratos"}`)

	// Simulate auth middleware: set user claims in Echo context
	c.Set("user_claims", &token.AccessClaims{
		UserID:    userID,
		Email:     "test@example.com",
		RoleID:    uuid.Nil,
		SessionID: uuid.Nil,
		JTI:       uuid.Nil,
	})

	err := handler.Handle(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the conversation was saved with the authenticated user ID
	found := false
	for _, conv := range convStore.convs {
		if conv.UserID == userID.String() {
			found = true
			break
		}
	}
	assert.True(t, found, "conversation should be saved with authenticated user_id")

	// Verify response includes conversation ID
	var resp ai_search.Response
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.ConversationID)
}

// TestHandler_InvalidJSON verifies that malformed JSON returns 400.
func TestHandler_InvalidJSON(t *testing.T) {
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		AIInterpreter:  &stubInterpreter{},
		FlightSearcher: &stubFlightSearcher{},
		HotelSearcher:  &stubHotelSearcher{},
		ConvStore:      &stubConversationStore{},
	})

	handler := ai_search.NewHandler(uc, nil, defaultCfg)

	c, rec := newEchoContext(`not json`)

	// Handler returns HTTPError(400) on bind failure.
	// In direct handler tests, the error is returned directly.
	err := handler.Handle(c)
	assert.Error(t, err)
	var he *echo.HTTPError
	if assert.ErrorAs(t, err, &he) {
		assert.Equal(t, http.StatusBadRequest, he.Code)
	}
	_ = rec
}
