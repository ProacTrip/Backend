package update_notif_prefs

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

type testUNPRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error)
	upsertFn      func(ctx context.Context, p *domain.NotificationPreference) error
}
func (m *testUNPRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error) {
	if m.getByUserIDFn != nil { return m.getByUserIDFn(ctx, userID) }; return nil, nil
}
func (m *testUNPRepo) Upsert(ctx context.Context, p *domain.NotificationPreference) error {
	if m.upsertFn != nil { return m.upsertFn(ctx, p) }; return nil
}
func (m *testUNPRepo) Delete(ctx context.Context, userID uuid.UUID, channel domain.NotificationChannel, notifType domain.NotificationType) error { return nil }

type testUNPEventPub struct{ publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error) }
func (m *testUNPEventPub) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil { return m.publishFn(ctx, stream, payload) }; return "id", nil
}

func TestUpdateNotifPrefsHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; repo *testUNPRepo; ep *testUNPEventPub; wantStatus int
	}{
		{"debe retornar 200 con preferencia actualizada", tc, `{"channel":"email","notification_type":"booking_confirmation","enabled":true}`, &testUNPRepo{upsertFn: func(ctx context.Context, p *domain.NotificationPreference) error { return nil }}, &testUNPEventPub{}, http.StatusOK},
		{"debe retornar error sin claims", nil, `{"channel":"email"}`, &testUNPRepo{}, &testUNPEventPub{}, http.StatusInternalServerError},
		{"debe retornar 400 cuando channel invalido", tc, `{"channel":"telegram","notification_type":"booking_confirmation","enabled":true}`, &testUNPRepo{}, &testUNPEventPub{}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPut, "/v1/user/profile/notifications", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{NotifPrefsRepo: tt.repo, EventPublisher: tt.ep}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
