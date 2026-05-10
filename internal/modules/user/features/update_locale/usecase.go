// Caso de uso: Actualizar locale del perfil (PUT /v1/user/profile/locale).
// Valida timezone IANA, lenguaje ISO 639, moneda ISO 4217.
package update_locale

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/features/shared"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Ports
// =============================================================================

type ProfileRepo interface {
	UpdateLocale(ctx context.Context, userID uuid.UUID, timezone, language, currency, currentLocation string) error
}

type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

type UseCaseDeps struct {
	ProfileRepo    ProfileRepo
	EventPublisher EventPublisher
	RedisClient    *redis.Client
}

type UseCase struct {
	profileRepo    ProfileRepo
	eventPublisher EventPublisher
	redisClient    *redis.Client
	wg             sync.WaitGroup
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		profileRepo:    deps.ProfileRepo,
		eventPublisher: deps.EventPublisher,
		redisClient:    deps.RedisClient,
	}
}

// Wait espera a que todos los eventos publicados asíncronamente terminen.
func (uc *UseCase) Wait() { uc.wg.Wait() }

// Execute valida los códigos de locale y actualiza.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) error {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	// Default a strings vacíos si no se enviaron
	tz := ""
	lang := ""
	cur := ""
	loc := ""

	// Validar timezone contra base IANA
	if cmd.TimezoneName != nil && *cmd.TimezoneName != "" {
		tz = *cmd.TimezoneName
		if _, err := time.LoadLocation(tz); err != nil {
			return domain.ErrInvalidTimezone
		}
	}

	// Validar language code: 2-5 caracteres (ISO 639-1 o ISO 639-2)
	if cmd.LanguageCode != nil && *cmd.LanguageCode != "" {
		lang = *cmd.LanguageCode
		if len(lang) < 2 || len(lang) > 5 {
			return domain.ErrInvalidLanguageCode
		}
	}

	// Validar currency code: exactamente 3 caracteres (ISO 4217)
	if cmd.CurrencyCode != nil && *cmd.CurrencyCode != "" {
		cur = *cmd.CurrencyCode
		if len(cur) != 3 {
			return domain.ErrInvalidCurrencyCode
		}
	}

	// current_location: cadena libre (city, country)
	if cmd.CurrentLocation != nil {
		loc = *cmd.CurrentLocation
	}

	if err := uc.profileRepo.UpdateLocale(ctx, userID, tz, lang, cur, loc); err != nil {
		return fmt.Errorf("update locale: %w", err)
	}

	// Invalidate prefs cache
	if uc.redisClient != nil {
		_ = shared.DeleteProfilePrefs(ctx, uc.redisClient, userID.String())
	}

	// Emit event (best-effort)
	if uc.eventPublisher != nil {
		uc.wg.Go(func() {
			bgCtx := context.WithoutCancel(ctx)
			_, err := uc.eventPublisher.Publish(bgCtx,
				eventbus.StreamName("user.locale.updated"),
				map[string]interface{}{
					"user_id":          userID.String(),
					"timezone":         tz,
					"language_code":    lang,
					"currency_code":    cur,
					"current_location": loc,
				},
			)
			if err != nil {
				slog.WarnContext(bgCtx, "publish locale updated event failed",
					slog.String("user_id", userID.String()),
					slog.String("error", err.Error()),
				)
			}
		})
	}

	return nil
}
