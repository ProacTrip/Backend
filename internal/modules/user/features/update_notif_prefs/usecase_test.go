// RED — Test del usecase update_notif_prefs.
package update_notif_prefs

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockNotifPrefsRepo struct {
	upsertFn func(ctx context.Context, p *domain.NotificationPreference) error
	deleteFn func(ctx context.Context, userID uuid.UUID, ch domain.NotificationChannel, nt domain.NotificationType) error
}

func (m *mockNotifPrefsRepo) Create(ctx context.Context, p *domain.NotificationPreference) error { return nil }
func (m *mockNotifPrefsRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error) {
	return nil, nil
}
func (m *mockNotifPrefsRepo) Upsert(ctx context.Context, p *domain.NotificationPreference) error {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, p)
	}
	return nil
}
func (m *mockNotifPrefsRepo) Delete(ctx context.Context, userID uuid.UUID, ch domain.NotificationChannel, nt domain.NotificationType) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, userID, ch, nt)
	}
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func TestUpdateNotifPrefs_UpsertEnabled(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	channel := "email"
	notifType := "price_alert"
	enabled := true

	called := false
	uc := NewUseCase(UseCaseDeps{
		NotifPrefsRepo: &mockNotifPrefsRepo{
			upsertFn: func(ctx context.Context, p *domain.NotificationPreference) error {
				called = true
				if p.Channel != domain.NotifChannelEmail {
					t.Errorf("Channel = %s, esperado email", p.Channel)
				}
				if p.NotificationType != domain.NotifTypePriceAlert {
					t.Errorf("NotificationType = %s, esperado price_alert", p.NotificationType)
				}
				if !p.Enabled {
					t.Error("Enabled debería ser true")
				}
				return nil
			},
		},
	})

	cmd := Command{
		UserID:           userID.String(),
		Channel:          channel,
		NotificationType: notifType,
		Enabled:          enabled,
	}
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("Upsert debería haber sido llamado")
	}
}

func TestUpdateNotifPrefs_DeleteWhenDisabled(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	channel := "websocket"
	notifType := "promo_offer"
	enabled := false

	called := false
	uc := NewUseCase(UseCaseDeps{
		NotifPrefsRepo: &mockNotifPrefsRepo{
			deleteFn: func(ctx context.Context, id uuid.UUID, ch domain.NotificationChannel, nt domain.NotificationType) error {
				called = true
				if ch != domain.NotifChannelWebSocket {
					t.Errorf("Channel = %s, esperado websocket", ch)
				}
				if nt != domain.NotifTypePromoOffer {
					t.Errorf("Type = %s, esperado promo_offer", nt)
				}
				return nil
			},
		},
	})

	cmd := Command{
		UserID:           userID.String(),
		Channel:          channel,
		NotificationType: notifType,
		Enabled:          enabled,
	}
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("Delete debería haber sido llamado cuando enabled=false")
	}
}

func TestUpdateNotifPrefs_InvalidChannel(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{NotifPrefsRepo: &mockNotifPrefsRepo{}})

	cmd := Command{
		UserID:           userID.String(),
		Channel:          "carrier_pigeon",
		NotificationType: "price_alert",
		Enabled:          true,
	}
	err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrInvalidChannel) {
		t.Errorf("se esperaba ErrInvalidChannel, obtuve %v", err)
	}
}

func TestUpdateNotifPrefs_ValidChannels(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name    string
		channel string
		valid   bool
	}{
		{"email", "email", true},
		{"sms (planned but accepted)", "sms", true},
		{"websocket", "websocket", true},
		{"invalid", "fax", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := Command{
				UserID:           userID.String(),
				Channel:          tc.channel,
				NotificationType: "price_alert",
				Enabled:          true,
			}
			uc := NewUseCase(UseCaseDeps{NotifPrefsRepo: &mockNotifPrefsRepo{}})
			err := uc.Execute(t.Context(), cmd)
			if tc.valid && err != nil {
				t.Errorf("no se esperaba error para %s: %v", tc.channel, err)
			}
			if !tc.valid && !errors.Is(err, domain.ErrInvalidChannel) {
				t.Errorf("se esperaba ErrInvalidChannel para %s, obtuve %v", tc.channel, err)
			}
		})
	}
}

func TestUpdateNotifPrefs_SMSAcceptedWithWarning(t *testing.T) {
	// SMS está planeado pero no implementado — el handler debe aceptarlo
	userID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{NotifPrefsRepo: &mockNotifPrefsRepo{}})

	cmd := Command{
		UserID:           userID.String(),
		Channel:          "sms",
		NotificationType: "travel_reminder",
		Enabled:          true,
	}
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Errorf("SMS debería ser aceptado aunque no esté implementado: %v", err)
	}
}
