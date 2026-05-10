package update_locale

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	// "github.com/ProacTrip/Backend/internal/modules/user/domain"
)

type testULProfileRepo struct{ updateLocaleFn func(ctx context.Context, userID uuid.UUID, tz, lang, cur, loc string) error }
func (m *testULProfileRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, tz, lang, cur, loc string) error {
	if m.updateLocaleFn != nil { return m.updateLocaleFn(ctx, userID, tz, lang, cur, loc) }; return nil
}
type testULEventPub struct{ publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error) }
func (m *testULEventPub) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil { return m.publishFn(ctx, stream, payload) }; return "id", nil
}

func TestUpdateLocaleHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; pr *testULProfileRepo; ep *testULEventPub; wantStatus int
	}{
		{"debe retornar 200 actualizado", tc, `{"timezone":"America/Argentina/Buenos_Aires","language":"es","currency":"ARS"}`, &testULProfileRepo{}, &testULEventPub{}, http.StatusOK},
		{"debe retornar error sin claims", nil, `{"timezone":"UTC"}`, &testULProfileRepo{}, &testULEventPub{}, http.StatusInternalServerError},
		{"debe retornar 200 con timezone custom", tc, `{"timezone":"Invalid/Zone"}`, &testULProfileRepo{}, &testULEventPub{}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPut, "/v1/user/profile/locale", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{ProfileRepo: tt.pr, EventPublisher: tt.ep, RedisClient: nil})
			h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
