package update_travel_prefs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

type testUTPTRepo struct {
	createFn      func(ctx context.Context, p *domain.TravelPreferences) error
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error)
	updateFn      func(ctx context.Context, p *domain.TravelPreferences) error
}
func (m *testUTPTRepo) Create(ctx context.Context, p *domain.TravelPreferences) error {
	if m.createFn != nil { return m.createFn(ctx, p) }; return nil
}
func (m *testUTPTRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error) {
	if m.getByUserIDFn != nil { return m.getByUserIDFn(ctx, userID) }; return nil, nil
}
func (m *testUTPTRepo) Update(ctx context.Context, p *domain.TravelPreferences) error {
	if m.updateFn != nil { return m.updateFn(ctx, p) }; return nil
}

type testUTPEventPub struct{ publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error) }
func (m *testUTPEventPub) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil { return m.publishFn(ctx, stream, payload) }; return "id", nil
}

func TestUpdateTravelPrefsHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; repo *testUTPTRepo; ep *testUTPEventPub; wantStatus int
	}{
		{"debe retornar 200 con preferencias actualizadas", tc, `{"preferred_class":"business","avoid_layovers":false}`, &testUTPTRepo{getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.TravelPreferences, error) {
			return domain.NewTravelPreferences(uid), nil
		}}, &testUTPEventPub{}, http.StatusOK},
		{"debe retornar error sin claims", nil, `{"preferred_class":"economy"}`, &testUTPTRepo{}, &testUTPEventPub{}, http.StatusInternalServerError},
		{"debe retornar 200 con preferred_class custom", tc, `{"preferred_class":"first"}`, &testUTPTRepo{}, &testUTPEventPub{}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPut, "/v1/user/profile/travel-preferences", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{TravelPrefsRepo: tt.repo, EventPublisher: tt.ep}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
