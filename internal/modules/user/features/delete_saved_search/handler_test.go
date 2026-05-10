package delete_saved_search

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

type testDelSRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}
func (m *testDelSRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
	if m.getByIDFn != nil { return m.getByIDFn(ctx, id) }; return nil, nil
}
func (m *testDelSRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil { return m.deleteFn(ctx, id) }; return nil
}

func TestDeleteSavedSearchHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	sid := uuid.Must(uuid.NewV7()).String()
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; repo *testDelSRepo; wantStatus int
	}{
		{"debe retornar 200 al eliminar", tc, &testDelSRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
			return &domain.SavedSearch{ID: uuid.Must(uuid.NewV7()), UserID: tc.UserID}, nil
		}, deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }}, http.StatusOK},
		{"debe retornar error cuando no hay claims", nil, &testDelSRepo{}, http.StatusInternalServerError},
		{"debe retornar error cuando no existe", tc, &testDelSRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
			return nil, domain.ErrSearchNotFound
		}}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodDelete, "/v1/user/saved-searches/"+sid, nil)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			c.SetPath("/v1/user/saved-searches/:search_id"); c.SetPathValues(echo.PathValues{{Name: "search_id", Value: sid}})
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{SavedSearchRepo: tt.repo}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
