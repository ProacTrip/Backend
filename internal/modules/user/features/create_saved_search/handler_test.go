package create_saved_search

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

type testCSSearchRepo struct{ createFn func(ctx context.Context, s *domain.SavedSearch) error }
func (m *testCSSearchRepo) Create(ctx context.Context, s *domain.SavedSearch) error {
	if m.createFn != nil { return m.createFn(ctx, s) }; return nil
}
func (m *testCSSearchRepo) GetByHash(ctx context.Context, userID uuid.UUID, hash string) (*domain.SavedSearch, error) { return nil, nil }

type testCSHashService struct{ hashFn func(data []byte) string }
func (m *testCSHashService) Hash(data []byte) string {
	if m.hashFn != nil { return m.hashFn(data) }; return "testhash"
}

func TestCreateSavedSearchHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; repo *testCSSearchRepo; hasher *testCSHashService; wantStatus int
	}{
		{"debe retornar 201 con busqueda creada", tc, `{"name":"test","origin":"EZE","destination":"MIA"}`, &testCSSearchRepo{createFn: func(ctx context.Context, s *domain.SavedSearch) error { return nil }}, &testCSHashService{}, http.StatusInternalServerError},
		{"debe retornar error cuando no hay claims", nil, `{"name":"test"}`, &testCSSearchRepo{}, &testCSHashService{}, http.StatusInternalServerError},
		{"debe retornar error cuando falta name", tc, `{}`, &testCSSearchRepo{}, &testCSHashService{}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPost, "/v1/user/saved-searches", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{SavedSearchRepo: tt.repo, HashService: tt.hasher}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
