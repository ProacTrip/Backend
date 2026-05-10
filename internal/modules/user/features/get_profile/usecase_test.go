// RED — Test del usecase get_profile.
// Referencia tipos que aún no existen (command, usecase, response).
package get_profile

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks — Implementaciones falsas de los repositorios
// =============================================================================

type mockProfileRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
}

func (m *mockProfileRepo) Create(ctx context.Context, p *domain.UserProfile) error  { return nil }
func (m *mockProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *mockProfileRepo) Update(ctx context.Context, p *domain.UserProfile) error    { return nil }
func (m *mockProfileRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, timezone, language, currency string) error {
	return nil
}
func (m *mockProfileRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	return nil
}

type mockTravelPrefsRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error)
}

func (m *mockTravelPrefsRepo) Create(ctx context.Context, p *domain.TravelPreferences) error {
	return nil
}
func (m *mockTravelPrefsRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockTravelPrefsRepo) Update(ctx context.Context, p *domain.TravelPreferences) error {
	return nil
}

type mockNotifPrefsRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error)
}

func (m *mockNotifPrefsRepo) Create(ctx context.Context, p *domain.NotificationPreference) error { return nil }
func (m *mockNotifPrefsRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockNotifPrefsRepo) Upsert(ctx context.Context, p *domain.NotificationPreference) error {
	return nil
}
func (m *mockNotifPrefsRepo) Delete(ctx context.Context, userID uuid.UUID, channel domain.NotificationChannel, notifType domain.NotificationType) error {
	return nil
}

// =============================================================================
// Tests — Table-driven
// =============================================================================

func TestGetProfile_HappyPath(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	profile := domain.NewUserProfile(userID, "test@example.com")
	profile.FirstName = new("María")
	profile.LastName = new("Gómez")
	profile.TimezoneName = "America/Argentina/Buenos_Aires"
	profile.LanguageCode = "es"
	profile.CurrencyCode = "ARS"

	travelPrefs := domain.NewTravelPreferences(userID)
	travelPrefs.PreferredClass = domain.CabinClassBusiness

	notifPrefs := []*domain.NotificationPreference{
		domain.NewNotificationPreference(userID, domain.NotifChannelEmail, domain.NotifTypeBookingConfirmation),
	}

	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return profile, nil
			},
		},
		TravelPrefsRepo: &mockTravelPrefsRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TravelPreferences, error) {
				return travelPrefs, nil
			},
		},
		NotifPrefsRepo: &mockNotifPrefsRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) ([]*domain.NotificationPreference, error) {
				return notifPrefs, nil
			},
		},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp == nil {
		t.Fatal("response no debería ser nil")
	}
	if resp.FirstName == nil || *resp.FirstName != "María" {
		t.Errorf("FirstName = %v, se esperaba María", resp.FirstName)
	}
	if resp.Location.Timezone != "America/Argentina/Buenos_Aires" {
		t.Errorf("Location.Timezone = %s, se esperaba America/Argentina/Buenos_Aires", resp.Location.Timezone)
	}
	if resp.TravelPreferences == nil {
		t.Fatal("TravelPreferences no debería ser nil")
	}
	if resp.TravelPreferences.PreferredClass != "business" {
		t.Errorf("PreferredClass = %s, se esperaba business", resp.TravelPreferences.PreferredClass)
	}
	if len(resp.NotificationPreferences) != 1 {
		t.Errorf("len(NotificationPreferences) = %d, se esperaba 1", len(resp.NotificationPreferences))
	}
}

func TestGetProfile_ProfileNotFound(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return nil, domain.ErrProfileNotFound
			},
		},
		TravelPrefsRepo: &mockTravelPrefsRepo{},
		NotifPrefsRepo:  &mockNotifPrefsRepo{},
	})

	cmd := Command{UserID: uuid.Must(uuid.NewV7()).String()}
	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error ErrProfileNotFound")
	}
	if !errors.Is(err, domain.ErrProfileNotFound) {
		t.Errorf("error = %v, se esperaba ErrProfileNotFound", err)
	}
}

func TestGetProfile_TravelPrefsAndNotifPrefsMayBeNil(t *testing.T) {
	// Si el usuario no tiene travel_prefs ni notif_prefs, deben ser nil
	userID := uuid.Must(uuid.NewV7())
	profile := domain.NewUserProfile(userID, "")

	uc := NewUseCase(UseCaseDeps{
		ProfileRepo:    &mockProfileRepo{getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) { return profile, nil }},
		TravelPrefsRepo: &mockTravelPrefsRepo{getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TravelPreferences, error) { return nil, domain.ErrTravelPrefsNotFound }},
		NotifPrefsRepo:  &mockNotifPrefsRepo{getByUserIDFn: func(ctx context.Context, id uuid.UUID) ([]*domain.NotificationPreference, error) { return nil, nil }},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.TravelPreferences != nil {
		t.Error("TravelPreferences debería ser nil cuando no hay datos")
	}
	if resp.NotificationPreferences != nil {
		t.Error("NotificationPreferences debería ser nil cuando no hay datos")
	}
}

func TestGetProfile_NotificationPreferencesGroupedByType(t *testing.T) {
	// Verifica que notification_preferences se agrupen por notification_type
	// y no por channel (USER_API.md § Get Profile)
	userID := uuid.Must(uuid.NewV7())
	profile := domain.NewUserProfile(userID, "test@example.com")

	// Tres preferencias: 2 tipos con distintos canales
	notifPrefs := []*domain.NotificationPreference{
		domain.NewNotificationPreference(userID, domain.NotifChannelEmail, domain.NotifTypeBookingConfirmation),
		domain.NewNotificationPreference(userID, domain.NotifChannelSMS, domain.NotifTypeBookingConfirmation),
		domain.NewNotificationPreference(userID, domain.NotifChannelEmail, domain.NotifTypePromotional),
	}

	// booking_confirmation: email=true, sms=true (websocket debe estar false)
	notifPrefs[0].Enabled = true
	notifPrefs[1].Enabled = true
	// promotional: email=true (sms y websocket deben estar false)
	notifPrefs[2].Enabled = true

	uc := NewUseCase(UseCaseDeps{
		ProfileRepo:    &mockProfileRepo{getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) { return profile, nil }},
		TravelPrefsRepo: &mockTravelPrefsRepo{getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TravelPreferences, error) { return nil, domain.ErrTravelPrefsNotFound }},
		NotifPrefsRepo: &mockNotifPrefsRepo{getByUserIDFn: func(ctx context.Context, id uuid.UUID) ([]*domain.NotificationPreference, error) { return notifPrefs, nil }},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// Debe tener 2 tipos de notificación (booking_confirmation y promotional)
	if len(resp.NotificationPreferences) != 2 {
		t.Fatalf("len(NotificationPreferences) = %d, se esperaba 2", len(resp.NotificationPreferences))
	}

	// booking_confirmation: email=true, sms=true, websocket=false
	booking, ok := resp.NotificationPreferences["booking_confirmation"]
	if !ok {
		t.Fatal("NotificationPreferences no contiene 'booking_confirmation'")
	}
	if !booking.Email {
		t.Error("booking_confirmation.Email debería ser true")
	}
	if !booking.SMS {
		t.Error("booking_confirmation.SMS debería ser true")
	}
	if booking.Websocket {
		t.Error("booking_confirmation.Websocket debería ser false (no configurado)")
	}

	// promotional: email=true, sms=false, websocket=false
	promo, ok := resp.NotificationPreferences["promotional"]
	if !ok {
		t.Fatal("NotificationPreferences no contiene 'promotional'")
	}
	if !promo.Email {
		t.Error("promotional.Email debería ser true")
	}
	if promo.SMS {
		t.Error("promotional.SMS debería ser false (no configurado)")
	}
	if promo.Websocket {
		t.Error("promotional.Websocket debería ser false (no configurado)")
	}

	// flight_reminder no debería estar presente (no configurado)
	if _, exists := resp.NotificationPreferences["flight_reminder"]; exists {
		t.Error("flight_reminder no debería aparecer (no configurado)")
	}
}
