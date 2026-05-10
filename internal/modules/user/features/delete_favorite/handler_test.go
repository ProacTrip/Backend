package delete_favorite

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

type testDelFavRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.Favorite, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}
func (m *testDelFavRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Favorite, error) {
	if m.getByIDFn != nil { return m.getByIDFn(ctx, id) }; return nil, nil
}
func (m *testDelFavRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil { return m.deleteFn(ctx, id) }; return nil
}

func TestDeleteFavoriteHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	fid := uuid.Must(uuid.NewV7()).String()
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; repo *testDelFavRepo; wantStatus int
	}{
		{"debe retornar 200 al eliminar favorito", tc, &testDelFavRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Favorite, error) {
			return &domain.Favorite{ID: uuid.Must(uuid.NewV7()), UserID: tc.UserID}, nil
		}, deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil }}, http.StatusOK},
		{"debe retornar error cuando no hay claims", nil, &testDelFavRepo{}, http.StatusInternalServerError},
		{"debe retornar error cuando favorito no existe", tc, &testDelFavRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Favorite, error) {
			return nil, domain.ErrFavoriteNotFound
		}}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodDelete, "/v1/user/favorites/"+fid, nil)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			c.SetPath("/v1/user/favorites/:favorite_id"); c.SetPathValues(echo.PathValues{{Name: "favorite_id", Value: fid}})
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{FavoriteRepo: tt.repo}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
