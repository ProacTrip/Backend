package toggle_alert

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

type testTASearchRepo struct {
	getByIDFn       func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error)
	setAlertEnabledFn func(ctx context.Context, id uuid.UUID, enabled bool) error
}
func (m *testTASearchRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
	if m.getByIDFn != nil { return m.getByIDFn(ctx, id) }; return nil, nil
}
func (m *testTASearchRepo) SetAlertEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	if m.setAlertEnabledFn != nil { return m.setAlertEnabledFn(ctx, id, enabled) }; return nil
}

func TestToggleAlertHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	sid := uuid.Must(uuid.NewV7()).String()
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; repo *testTASearchRepo; wantStatus int
	}{
		{"debe retornar 200 con alerta toggled", tc, `{"enabled":true}`, &testTASearchRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
			return &domain.SavedSearch{ID: uuid.Must(uuid.NewV7()), UserID: tc.UserID}, nil
		}}, http.StatusOK},
		{"debe retornar error sin claims", nil, `{"enabled":true}`, &testTASearchRepo{}, http.StatusInternalServerError},
		{"debe retornar error cuando no existe", tc, `{"enabled":false}`, &testTASearchRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
			return nil, domain.ErrSearchNotFound
		}}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPut, "/v1/user/saved-searches/"+sid+"/alert", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			c.SetPath("/v1/user/saved-searches/:search_id/alert"); c.SetPathValues(echo.PathValues{{Name: "search_id", Value: sid}})
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{SavedSearchRepo: tt.repo}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
