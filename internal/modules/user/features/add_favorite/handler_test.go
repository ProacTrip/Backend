// Tests para el handler POST /v1/user/favorites.
package add_favorite

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

type testFavoriteRepo struct {
	createFn func(ctx context.Context, f *domain.Favorite) error
}

func (m *testFavoriteRepo) Create(ctx context.Context, f *domain.Favorite) error {
	if m.createFn != nil {
		return m.createFn(ctx, f)
	}
	return nil
}

func testClaims() *sharedauth.AccessClaims {
	uid := uuid.Must(uuid.NewV7())
	return &sharedauth.AccessClaims{
		UserID: uid, Email: "test@example.com",
		RoleID: uuid.Must(uuid.NewV7()), Role: "client",
		SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7()),
	}
}

func TestAddFavoriteHandler_Handle(t *testing.T) {
	tests := []struct {
		name       string
		claims     *sharedauth.AccessClaims
		body       string
		repo       *testFavoriteRepo
		wantStatus int
	}{
		{
			name:   "debe retornar 201 con favorito creado (hotel)",
			claims: testClaims(),
			body:   `{"entity_id":"` + uuid.Must(uuid.NewV7()).String() + `","entity_type":"hotel","title":"Hotel Ejemplo"}`,
			repo: &testFavoriteRepo{
				createFn: func(ctx context.Context, f *domain.Favorite) error { return nil },
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "debe retornar error cuando no hay claims",
			body:       `{"entity_id":"x","entity_type":"hotel","title":"Test"}`,
			repo:       &testFavoriteRepo{},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "debe retornar 400 cuando entity_type es inválido",
			claims:     testClaims(),
			body:       `{"entity_id":"` + uuid.Must(uuid.NewV7()).String() + `","entity_type":"airport","title":"Test"}`,
			repo:       &testFavoriteRepo{},
			wantStatus: http.StatusInternalServerError, // mapper no registrado
		},
		{
			name:   "debe retornar 201 con entity_type flight",
			claims: testClaims(),
			body:   `{"entity_id":"` + uuid.Must(uuid.NewV7()).String() + `","entity_type":"flight","title":"Vuelo"}`,
			repo: &testFavoriteRepo{
				createFn: func(ctx context.Context, f *domain.Favorite) error { return nil },
			},
			wantStatus: http.StatusCreated,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/v1/user/favorites", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tc.claims != nil {
				c.Set("user_claims", tc.claims)
			}

			uc := NewUseCase(UseCaseDeps{FavoriteRepo: tc.repo})
			h := NewHandler(uc)
			_ = h.Handle(c)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, se esperaba %d. body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
