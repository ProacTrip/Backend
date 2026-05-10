package list_favorites

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

type testLFavRepo struct{ getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]*domain.Favorite, error) }
func (m *testLFavRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Favorite, error) {
	if m.getByUserIDFn != nil { return m.getByUserIDFn(ctx, userID) }; return nil, nil
}

func TestListFavoritesHandler_Handle(t *testing.T) {
	uid := uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	tests := []struct {
		name string; claims *sharedauth.AccessClaims; repo *testLFavRepo; wantStatus int
	}{
		{"debe retornar 200 con lista de favoritos", tc, &testLFavRepo{getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.Favorite, error) { return []*domain.Favorite{}, nil }}, http.StatusOK},
		{"debe retornar error sin claims", nil, &testLFavRepo{}, http.StatusInternalServerError},
		{"debe retornar error cuando repo falla", tc, &testLFavRepo{getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.Favorite, error) { return nil, domain.ErrFavoriteNotFound }}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New(); req := httptest.NewRequest(http.MethodGet, "/v1/user/favorites", nil)
			rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
			if tt.claims != nil { c.Set("user_claims", tt.claims) }
			uc := NewUseCase(UseCaseDeps{FavoriteRepo: tt.repo}); h := NewHandler(uc); _ = h.Handle(c)
			if rec.Code != tt.wantStatus { t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus) }
		})
	}
}
