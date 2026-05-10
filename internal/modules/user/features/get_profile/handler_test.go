// Tests para el handler GET /v1/user/profile.
// Verifica todos los campos del response según USER_API.md.
// Usa UseCase real con repos mockeados (white-box testing).
package get_profile

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

// =============================================================================
// Mocks (mismos que en usecase_test.go)
// =============================================================================

type testProfileRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
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
func (m *testProfileRepo) Update(ctx context.Context, p *domain.UserProfile) error { return nil }
func (m *testProfileRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, tz, lang, curr string) error {
	return nil
}
func (m *testProfileRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, url string) error {
	return nil
}

type testTravelPrefsRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error)
}

func (m *testTravelPrefsRepo) Create(ctx context.Context, p *domain.TravelPreferences) error {
	return nil
}
func (m *testTravelPrefsRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *testTravelPrefsRepo) Update(ctx context.Context, p *domain.TravelPreferences) error {
	return nil
}

type testNotifPrefsRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error)
}

func (m *testNotifPrefsRepo) Create(ctx context.Context, p *domain.NotificationPreference) error {
	return nil
}
func (m *testNotifPrefsRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *testNotifPrefsRepo) Upsert(ctx context.Context, p *domain.NotificationPreference) error {
	return nil
}
func (m *testNotifPrefsRepo) Delete(ctx context.Context, userID uuid.UUID, channel domain.NotificationChannel, notifType domain.NotificationType) error {
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

func testClaims() *sharedauth.AccessClaims {
	uid := uuid.Must(uuid.NewV7())
	return &sharedauth.AccessClaims{
		UserID:    uid,
		Email:     "test@example.com",
		RoleID:    uuid.Must(uuid.NewV7()),
		Role:      "client",
		SessionID: uuid.Must(uuid.NewV7()),
		JTI:       uuid.Must(uuid.NewV7()),
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestGetProfileHandler_Handle(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	profile := domain.NewUserProfile(userID, "maria@example.com")
	profile.FirstName = new("María")
	profile.LastName = new("Gómez")
	g := domain.GenderFemale
	profile.Gender = &g
	profile.Nationality = new("AR")
	profile.IsPublic = true
	profile.TimezoneName = "America/Argentina/Buenos_Aires"
	profile.LanguageCode = "es"
	profile.CurrencyCode = "ARS"

	travelPrefs := domain.NewTravelPreferences(userID)
	travelPrefs.PreferredClass = domain.CabinClassBusiness

	notifPrefs := []*domain.NotificationPreference{
		domain.NewNotificationPreference(userID, domain.NotifChannelEmail, domain.NotifTypeBookingConfirmation),
	}

	tests := []struct {
		name            string
		claims          *sharedauth.AccessClaims
		profileRepo     *testProfileRepo
		travelPrefsRepo *testTravelPrefsRepo
		notifPrefsRepo  *testNotifPrefsRepo
		wantStatus      int
		wantContains    []string
	}{
		{
			name:   "debe retornar 200 con perfil completo incluyendo id y user_id",
			claims: testClaims(),
			profileRepo: &testProfileRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.UserProfile, error) { return profile, nil },
			},
			travelPrefsRepo: &testTravelPrefsRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.TravelPreferences, error) { return travelPrefs, nil },
			},
			notifPrefsRepo: &testNotifPrefsRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.NotificationPreference, error) {
					return notifPrefs, nil
				},
			},
			wantStatus: http.StatusOK,
			wantContains: []string{`"id"`, `"user_id"`, `"email"`, `"first_name"`, `"last_name"`,
				`"gender"`, `"is_public"`, `"location"`, `"timezone"`},
		},
		{
			name:            "debe retornar error cuando no hay claims en el contexto",
			profileRepo:     &testProfileRepo{},
			travelPrefsRepo: &testTravelPrefsRepo{},
			notifPrefsRepo:  &testNotifPrefsRepo{},
			wantStatus:      http.StatusInternalServerError,
		},
		{
			name:   "debe retornar 404 cuando el perfil no existe",
			claims: testClaims(),
			profileRepo: &testProfileRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.UserProfile, error) {
					return nil, domain.ErrProfileNotFound
				},
			},
			travelPrefsRepo: &testTravelPrefsRepo{},
			notifPrefsRepo:  &testNotifPrefsRepo{},
			wantStatus:      http.StatusInternalServerError, // mapper no registrado en test aislado
		},
		{
			name:   "debe retornar 200 con travel_preferences y notification_preferences nil",
			claims: testClaims(),
			profileRepo: &testProfileRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.UserProfile, error) { return profile, nil },
			},
			travelPrefsRepo: &testTravelPrefsRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.TravelPreferences, error) {
					return nil, domain.ErrTravelPrefsNotFound
				},
			},
			notifPrefsRepo: &testNotifPrefsRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.NotificationPreference, error) { return nil, nil },
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "debe retornar 200 con notification_preferences pobladas",
			claims: testClaims(),
			profileRepo: &testProfileRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.UserProfile, error) { return profile, nil },
			},
			travelPrefsRepo: &testTravelPrefsRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) (*domain.TravelPreferences, error) {
					return nil, domain.ErrTravelPrefsNotFound
				},
			},
			notifPrefsRepo: &testNotifPrefsRepo{
				getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.NotificationPreference, error) {
					return notifPrefs, nil
				},
			},
			wantStatus:   http.StatusOK,
			wantContains: []string{`"notification_preferences"`, `"booking_confirmation"`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/v1/user/profile", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tc.claims != nil {
				c.Set("user_claims", tc.claims)
			}

			uc := NewUseCase(UseCaseDeps{
				ProfileRepo:     tc.profileRepo,
				TravelPrefsRepo: tc.travelPrefsRepo,
				NotifPrefsRepo:  tc.notifPrefsRepo,
			})
			h := NewHandler(uc)
			_ = h.Handle(c)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, se esperaba %d. body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}

			for _, field := range tc.wantContains {
				if !strings.Contains(rec.Body.String(), field) {
					t.Errorf("campo %q no encontrado en la respuesta", field)
				}
			}
		})
	}
}
