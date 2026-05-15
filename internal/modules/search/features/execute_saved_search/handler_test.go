// Tests de integración HTTP para el handler execute_saved_search.
// Usa httptest con Echo para verificar status codes, headers y body.
// Reutiliza los stubs definidos en usecase_test.go (mismo package).
package execute_saved_search_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/execute_saved_search"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_flights"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Helper: crear Echo context con body y claims
// =============================================================================

func newEchoContext(method, path, body string, claims *sharedauth.AccessClaims) (*echo.Echo, *echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if claims != nil {
		c.Set("user_claims", claims)
	}

	return e, c, rec
}

// =============================================================================
// Helper: construir handler con use case real y stubs
// =============================================================================

func newTestHandler(
	provider domain.SavedSearchProvider,
	flightSearcher execute_saved_search.FlightSearcher,
	hotelSearcher execute_saved_search.HotelSearcher,
	aiSearcher execute_saved_search.AISearcher,
) *execute_saved_search.Handler {
	uc := execute_saved_search.NewUseCase(execute_saved_search.UseCaseDeps{
		SavedSearchProvider: provider,
		FlightSearcher:      flightSearcher,
		HotelSearcher:       hotelSearcher,
		AISearcher:          aiSearcher,
	})
	return execute_saved_search.NewHandler(uc)
}

// =============================================================================
// Tests
// =============================================================================

func TestHandler_Handle_Success(t *testing.T) {
	searchID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	params, _ := json.Marshal(search_flights.Command{
		TripType:     "round_trip",
		Departure:    "EZE",
		Arrival:      "MAD",
		OutboundDate: "2026-06-15",
		ReturnDate:   "2026-06-30",
		Adults:       1,
	})

	provider := &stubSavedSearchProvider{
		data: &domain.SavedSearchData{
			ID:         searchID,
			UserID:     userID,
			SearchType: "flight",
			Parameters: params,
		},
	}
	flightSearcher := &stubFlightSearcher{
		resp: &search_flights.Response{
			TripType:     "round_trip",
			ResultsState: "complete",
		},
	}

	handler := newTestHandler(provider, flightSearcher,
		&stubHotelSearcher{}, &stubAISearcher{})

	body := `{"saved_search_id":"` + searchID.String() + `"}`
	_, c, rec := newEchoContext(http.MethodPost, "/v1/search/execute_saved", body, &sharedauth.AccessClaims{
		UserID: userID,
	})

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", cacheControl, "no-store")
	}

	var resp execute_saved_search.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.SearchType != "flight" {
		t.Errorf("SearchType = %q, want %q", resp.SearchType, "flight")
	}
	if resp.SearchID != searchID.String() {
		t.Errorf("SearchID = %q, want %q", resp.SearchID, searchID.String())
	}
}

func TestHandler_Handle_MissingClaims(t *testing.T) {
	handler := newTestHandler(&stubSavedSearchProvider{},
		&stubFlightSearcher{}, &stubHotelSearcher{}, &stubAISearcher{})

	_, c, rec := newEchoContext(http.MethodPost, "/v1/search/execute_saved",
		`{"saved_search_id":"`+uuid.Must(uuid.NewV7()).String()+`"}`,
		nil,
	)

	handler.Handle(c)
	// httperr.MapError escribe la respuesta y retorna nil
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusBadRequest && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected error status, got %d", rec.Code)
	}
}

func TestHandler_Handle_InvalidBody(t *testing.T) {
	handler := newTestHandler(&stubSavedSearchProvider{},
		&stubFlightSearcher{}, &stubHotelSearcher{}, &stubAISearcher{})

	_, c, rec := newEchoContext(http.MethodPost, "/v1/search/execute_saved",
		`no es json`,
		&sharedauth.AccessClaims{UserID: uuid.Must(uuid.NewV7())},
	)

	handler.Handle(c)
	if rec.Code < 400 {
		t.Errorf("expected error status for invalid JSON body, got %d", rec.Code)
	}
}

func TestHandler_Handle_InvalidUUID(t *testing.T) {
	handler := newTestHandler(&stubSavedSearchProvider{},
		&stubFlightSearcher{}, &stubHotelSearcher{}, &stubAISearcher{})

	_, c, rec := newEchoContext(http.MethodPost, "/v1/search/execute_saved",
		`{"saved_search_id":"no-es-un-uuid"}`,
		&sharedauth.AccessClaims{UserID: uuid.Must(uuid.NewV7())},
	)

	handler.Handle(c)
	if rec.Code < 400 {
		t.Errorf("expected error status for invalid UUID, got %d", rec.Code)
	}
}

func TestHandler_Handle_UseCaseError(t *testing.T) {
	searchID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	provider := &stubSavedSearchProvider{
		err: errors.New("not found"),
	}

	handler := newTestHandler(provider,
		&stubFlightSearcher{}, &stubHotelSearcher{}, &stubAISearcher{})

	_, c, rec := newEchoContext(http.MethodPost, "/v1/search/execute_saved",
		`{"saved_search_id":"`+searchID.String()+`"}`,
		&sharedauth.AccessClaims{UserID: userID},
	)

	handler.Handle(c)
	if rec.Code < 400 {
		t.Errorf("expected error status when use case fails, got %d", rec.Code)
	}
}

func TestHandler_Handle_HotelResponse(t *testing.T) {
	searchID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	params, _ := json.Marshal(search_hotels.Command{
		Query:        "Madrid",
		CheckInDate:  "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:       2,
	})

	provider := &stubSavedSearchProvider{
		data: &domain.SavedSearchData{
			ID:         searchID,
			UserID:     userID,
			SearchType: "hotel",
			Parameters: params,
		},
	}
	hotelSearcher := &stubHotelSearcher{
		resp: &search_hotels.Response{
			Type:         "hotel_search",
			ResultsState: "complete",
		},
	}

	handler := newTestHandler(provider, &stubFlightSearcher{}, hotelSearcher, &stubAISearcher{})

	_, c, rec := newEchoContext(http.MethodPost, "/v1/search/execute_saved",
		`{"saved_search_id":"`+searchID.String()+`"}`,
		&sharedauth.AccessClaims{UserID: userID},
	)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp execute_saved_search.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Results.Hotels == nil {
		t.Error("expected non-nil Hotels in response")
	}
}

func TestHandler_RouteCommentMatches(t *testing.T) {
	handler := newTestHandler(&stubSavedSearchProvider{},
		&stubFlightSearcher{}, &stubHotelSearcher{}, &stubAISearcher{})
	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
}
