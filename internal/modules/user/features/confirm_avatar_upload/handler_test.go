package confirm_avatar_upload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

type testConfirmStorage struct {
	existsFn func(ctx context.Context, bucket, key string) (bool, error)
}
func (m *testConfirmStorage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	if m.existsFn != nil { return m.existsFn(ctx, bucket, key) }; return true, nil
}
type testConfirmEventPub struct{ publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error) }
func (m *testConfirmEventPub) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil { return m.publishFn(ctx, stream, payload) }; return "id", nil
}

func TestConfirmAvatarUploadHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; storage *testConfirmStorage; ep *testConfirmEventPub; wantStatus int
	}{
		{"debe retornar 202 con confirmacion aceptada", tc, `{"storage_key":"`+uuid.Must(uuid.NewV7()).String()+`"}`, &testConfirmStorage{}, &testConfirmEventPub{}, http.StatusAccepted},
		{"debe retornar error cuando no hay claims", nil, `{"storage_key":"test"}`, &testConfirmStorage{}, &testConfirmEventPub{}, http.StatusInternalServerError},
		{"debe retornar error cuando storage_key vacio", tc, `{}`, &testConfirmStorage{}, &testConfirmEventPub{}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPost, "/v1/user/profile/avatar/confirm", strings.NewReader(tt.body)); req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{Storage: tt.storage, EventPublisher: tt.ep}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
