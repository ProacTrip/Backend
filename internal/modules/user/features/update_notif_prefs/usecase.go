// Caso de uso: Actualizar preferencia de notificación (PUT /v1/user/profile/notifications).
// Siempre hace upsert — enabled=false preserva el registro para audit trail.
package update_notif_prefs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Ports
// =============================================================================

type NotifPrefsRepo interface {
	Upsert(ctx context.Context, pref *domain.NotificationPreference) error
	Delete(ctx context.Context, userID uuid.UUID, channel domain.NotificationChannel, notifType domain.NotificationType) error
}

type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

type UseCaseDeps struct {
	NotifPrefsRepo  NotifPrefsRepo
	EventPublisher  EventPublisher
}

type UseCase struct {
	notifPrefsRepo  NotifPrefsRepo
	eventPublisher  EventPublisher
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		notifPrefsRepo: deps.NotifPrefsRepo,
		eventPublisher: deps.EventPublisher,
	}
}

// Execute valida el canal y tipo de notificación, luego hace upsert o delete.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) error {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	// Validar canal
	channel, err := parseChannel(cmd.Channel)
	if err != nil {
		return err
	}

	// Validar tipo de notificación
	notifType := domain.NotificationType(cmd.NotificationType)
	if !IsValidNotificationType(notifType) {
		return domain.ErrInvalidNotificationType
	}

	// SMS está planeado pero no implementado — loggear advertencia
	if channel == domain.NotifChannelSMS {
		slog.WarnContext(ctx, "SMS notification channel is planned but not yet implemented",
			slog.String("user_id", userID.String()),
		)
	}

	// Upsert siempre — enabled=false preserva el registro para audit trail
	// en lugar de borrarlo (delete destruye el historial)
	pref := domain.NewNotificationPreference(userID, channel, notifType)
	pref.Enabled = cmd.Enabled
	if err := uc.notifPrefsRepo.Upsert(ctx, pref); err != nil {
		return fmt.Errorf("upsert notification preference: %w", err)
	}

	// Emit event (best-effort)
	if uc.eventPublisher != nil {
		go func() {
			bgCtx := context.WithoutCancel(ctx)
			_, err := uc.eventPublisher.Publish(bgCtx,
				eventbus.StreamName("user.notification_preferences.updated"),
				map[string]interface{}{
					"user_id": userID.String(),
				},
			)
			if err != nil {
				slog.WarnContext(bgCtx, "publish notification preferences updated event failed",
					slog.String("user_id", userID.String()),
					slog.String("error", err.Error()),
				)
			}
		}()
	}

	return nil
}

// parseChannel convierte string a NotificationChannel y valida.
func parseChannel(s string) (domain.NotificationChannel, error) {
	ch := domain.NotificationChannel(s)
	switch ch {
	case domain.NotifChannelEmail, domain.NotifChannelSMS, domain.NotifChannelWebSocket:
		return ch, nil
	default:
		return "", domain.ErrInvalidChannel
	}
}

var validNotifTypes = map[domain.NotificationType]bool{
	domain.NotifTypePriceAlert:          true,
	domain.NotifTypeBookingConfirm:      true,
	domain.NotifTypeTravelReminder:      true,
	domain.NotifTypePromoOffer:          true,
	domain.NotifTypeBookingConfirmation: true,
	domain.NotifTypeFlightReminder:      true,
	domain.NotifTypePromotional:         true,
}

// IsValidNotificationType valida que el tipo de notificación esté entre los valores permitidos.
func IsValidNotificationType(t domain.NotificationType) bool {
	return validNotifTypes[t]
}
