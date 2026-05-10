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
	notifType := "booking_confirmation"
	enabled := true

	called := false
	uc := NewUseCase(UseCaseDeps{
		NotifPrefsRepo: &mockNotifPrefsRepo{
			upsertFn: func(ctx context.Context, p *domain.NotificationPreference) error {
				called = true
				if p.Channel != domain.NotifChannelEmail {
					t.Errorf("Channel = %s, esperado email", p.Channel)
				}
				if p.NotificationType != domain.NotifTypeBookingConfirmation {
					t.Errorf("NotificationType = %s, esperado booking_confirmation", p.NotificationType)
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

func TestUpdateNotifPrefs_DisabledUpsertsWithFalse(t *testing.T) {
	// enabled=false mantiene el registro para audit trail — siempre hace upsert
	userID := uuid.Must(uuid.NewV7())
	channel := "websocket"
	notifType := "promotional"
	enabled := false

	called := false
	uc := NewUseCase(UseCaseDeps{
		NotifPrefsRepo: &mockNotifPrefsRepo{
			upsertFn: func(ctx context.Context, p *domain.NotificationPreference) error {
				called = true
				if p.Channel != domain.NotifChannelWebSocket {
					t.Errorf("Channel = %s, esperado websocket", p.Channel)
				}
				if p.NotificationType != domain.NotifTypePromotional {
					t.Errorf("Type = %s, esperado promotional", p.NotificationType)
				}
				if p.Enabled {
					t.Error("Enabled debería ser false")
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
		t.Error("Upsert debería haber sido llamado (enabled=false preserva audit trail)")
	}
}

func TestUpdateNotifPrefs_InvalidChannel(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{NotifPrefsRepo: &mockNotifPrefsRepo{}})

	cmd := Command{
		UserID:           userID.String(),
		Channel:          "carrier_pigeon",
			NotificationType: "booking_confirmation",
		Enabled:          true,
	}
	err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrInvalidEnum) {
		t.Errorf("se esperaba ErrInvalidEnum, obtuve %v", err)
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
		NotificationType: "booking_confirmation",
				Enabled:          true,
			}
			uc := NewUseCase(UseCaseDeps{NotifPrefsRepo: &mockNotifPrefsRepo{}})
			err := uc.Execute(t.Context(), cmd)
			if tc.valid && err != nil {
				t.Errorf("no se esperaba error para %s: %v", tc.channel, err)
			}
			if !tc.valid && !errors.Is(err, domain.ErrInvalidEnum) {
				t.Errorf("se esperaba ErrInvalidEnum para %s, obtuve %v", tc.channel, err)
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
		NotificationType: "flight_reminder",
		Enabled:          true,
	}
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Errorf("SMS debería ser aceptado aunque no esté implementado: %v", err)
	}
}
