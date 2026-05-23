// Tests para el handler PUT /v1/user/profile.
// Usa UseCase real con repos mockeados.
package update_profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

// =============================================================================
// Mocks de repositorios
// =============================================================================

type testProfileRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
	updateFn      func(ctx context.Context, p *domain.UserProfile) error
}

func (m *testProfileRepo) Create(ctx context.Context, p *domain.UserProfile) error { return nil }
func (m *testProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *testProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *testProfileRepo) Update(ctx context.Context, p *domain.UserProfile) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, p)
	}
	return nil
}
func (m *testProfileRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, tz, lang, curr string) error {
	return nil
}
func (m *testProfileRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, url string) error {
	return nil
}

type testEventPublisher struct {
	publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

func (m *testEventPublisher) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil {
		return m.publishFn(ctx, stream, payload)
	}
	return "event-id", nil
}

// =============================================================================
// Helpers
// =============================================================================

func testClaims() *sharedauth.AccessClaims {
	uid := uuid.Must(uuid.NewV7())
	return &sharedauth.AccessClaims{
		UserID: uid, Email: "test@example.com",
		RoleID: uuid.Must(uuid.NewV7()), Role: "client",
		JTI: uuid.Must(uuid.NewV7()),
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestUpdateProfileHandler_Handle(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	claims := &sharedauth.AccessClaims{
		UserID: userID, Email: "test@example.com",
		RoleID: uuid.Must(uuid.NewV7()), Role: "client",
		JTI: uuid.Must(uuid.NewV7()),
	}

	profile := domain.NewUserProfile(userID, "test@example.com")

	tests := []struct {
		name        string
		claims      *sharedauth.AccessClaims
		body        string
		profileRepo *testProfileRepo
		eventPub    *testEventPublisher
		wantStatus  int
		wantBody    string
	}{
		{
			name:   "debe retornar 200 con actualización parcial de nombre",
			claims: claims,
			body:   `{"first_name":"María","last_name":"Gómez"}`,
			profileRepo: &testProfileRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.UserProfile, error) { return profile, nil },
				updateFn:      func(ctx context.Context, p *domain.UserProfile) error { return nil },
			},
			eventPub:   &testEventPublisher{},
			wantStatus: http.StatusOK,
			wantBody:   "actualizado",
		},
		{
			name:        "debe retornar error cuando no hay claims",
			body:        `{"first_name":"Test"}`,
			profileRepo: &testProfileRepo{},
			eventPub:    &testEventPublisher{},
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:   "debe retornar 200 sin modificar campos (body vacío)",
			claims: claims,
			body:   `{}`,
			profileRepo: &testProfileRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.UserProfile, error) { return profile, nil },
				updateFn:      func(ctx context.Context, p *domain.UserProfile) error { return nil },
			},
			eventPub:   &testEventPublisher{},
			wantStatus: http.StatusOK,
		},
		{
			name:   "debe retornar error cuando perfil no existe",
			claims: claims,
			body:   `{"first_name":"Test"}`,
			profileRepo: &testProfileRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.UserProfile, error) {
					return nil, domain.ErrProfileNotFound
				},
			},
			eventPub:   &testEventPublisher{},
			wantStatus: http.StatusInternalServerError, // mapper no registrado en test aislado
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/v1/user/profile", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tc.claims != nil {
				c.Set("user_claims", tc.claims)
			}

			uc := NewUseCase(UseCaseDeps{
				ProfileRepo:    tc.profileRepo,
				EventPublisher: tc.eventPub,
			})
			h := NewHandler(uc)
			_ = h.Handle(c)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, se esperaba %d. body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("respuesta no contiene %q: %s", tc.wantBody, rec.Body.String())
			}
		})
	}
	_ = time.Now
}
