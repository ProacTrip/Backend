package update_saved_search

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

type testUSSearchRepo struct {
	getByIDFn  func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error)
	getByHashFn func(ctx context.Context, userID uuid.UUID, hash string) (*domain.SavedSearch, error)
	updateFn   func(ctx context.Context, s *domain.SavedSearch) error
}
func (m *testUSSearchRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
	if m.getByIDFn != nil { return m.getByIDFn(ctx, id) }; return nil, nil
}
func (m *testUSSearchRepo) GetByHash(ctx context.Context, userID uuid.UUID, hash string) (*domain.SavedSearch, error) {
	if m.getByHashFn != nil { return m.getByHashFn(ctx, userID, hash) }; return nil, nil
}
func (m *testUSSearchRepo) Update(ctx context.Context, s *domain.SavedSearch) error {
	if m.updateFn != nil { return m.updateFn(ctx, s) }; return nil
}

type testUSHashService struct{ hashFn func(data []byte) string }
func (m *testUSHashService) Hash(data []byte) string {
	if m.hashFn != nil { return m.hashFn(data) }; return "testhash"
}

func TestUpdateSavedSearchHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	sid := uuid.Must(uuid.NewV7()).String()
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; body string; repo *testUSSearchRepo; hasher *testUSHashService; wantStatus int
	}{
		{"debe retornar 200 actualizado", tc, `{"name":"updated"}`, &testUSSearchRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
			return &domain.SavedSearch{ID: uuid.Must(uuid.NewV7()), UserID: tc.UserID}, nil
		}}, &testUSHashService{}, http.StatusOK},
		{"debe retornar error sin claims", nil, `{"name":"test"}`, &testUSSearchRepo{}, &testUSHashService{}, http.StatusInternalServerError},
		{"debe retornar error cuando no existe", tc, `{"name":"test"}`, &testUSSearchRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
			return nil, domain.ErrSearchNotFound
		}}, &testUSHashService{}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodPut, "/v1/user/saved-searches/"+sid, strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			c.SetPath("/v1/user/saved-searches/:search_id"); c.SetPathValues(echo.PathValues{{Name: "search_id", Value: sid}})
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{SavedSearchRepo: tt.repo, HashService: tt.hasher}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
