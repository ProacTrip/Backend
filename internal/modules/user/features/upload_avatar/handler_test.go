package upload_avatar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	// "github.com/ProacTrip/Backend/internal/modules/user/domain"
)

type testUAStorage struct {
	generateUploadURLFn func(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}
func (m *testUAStorage) GenerateUploadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	if m.generateUploadURLFn != nil { return m.generateUploadURLFn(ctx, bucket, key, expiry) }; return "https://r2.example.com/upload", nil
}

func TestUploadAvatarHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; storage *testUAStorage; wantStatus int
	}{
		{"debe retornar 201 con URL prefirmada", tc, `{"file_name":"avatar.png","content_type":"image/png"}`, &testUAStorage{}, http.StatusInternalServerError},
		{"debe retornar error sin claims", nil, `{"file_name":"test.png"}`, &testUAStorage{}, http.StatusUnauthorized},
		{"debe retornar 400 cuando content_type invalido", tc, `{"file_name":"test.exe","content_type":"application/x-msdownload"}`, &testUAStorage{}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPost, "/v1/user/profile/avatar", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{Storage: tt.storage}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
