package list_saved_searches

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

type testLSSearchRepo struct{ getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error) }
func (m *testLSSearchRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error) {
	if m.getByUserIDFn != nil { return m.getByUserIDFn(ctx, userID) }; return nil, nil
}

func TestListSavedSearchesHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; repo *testLSSearchRepo; wantStatus int
	}{
		{"debe retornar 200 con lista", tc, &testLSSearchRepo{getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.SavedSearch, error) { return []*domain.SavedSearch{}, nil }}, http.StatusOK},
		{"debe retornar error sin claims", nil, &testLSSearchRepo{}, http.StatusInternalServerError},
		{"debe retornar error cuando repo falla", tc, &testLSSearchRepo{getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.SavedSearch, error) { return nil, domain.ErrSearchNotFound }}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodGet, "/v1/user/saved-searches", nil)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{SavedSearchRepo: tt.repo}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
