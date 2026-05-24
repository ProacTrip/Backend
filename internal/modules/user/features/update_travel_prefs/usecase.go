// Caso de uso: Actualizar preferencias de viaje (PATCH /v1/user/profile/travel-preferences).
// Si no existen, las crea. Si existen, actualiza los campos no-nil.
package update_travel_prefs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Ports
// =============================================================================

type TravelPrefsRepo interface {
	Create(ctx context.Context, prefs *domain.TravelPreferences) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error)
	Update(ctx context.Context, prefs *domain.TravelPreferences) error
}

type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

type UseCaseDeps struct {
	TravelPrefsRepo TravelPrefsRepo
	EventPublisher  EventPublisher
}

type UseCase struct {
	travelPrefsRepo TravelPrefsRepo
	eventPublisher  EventPublisher
	wg              sync.WaitGroup
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		travelPrefsRepo: deps.TravelPrefsRepo,
		eventPublisher:  deps.EventPublisher,
	}
}

// Wait espera a que todos los eventos publicados asíncronamente terminen.
func (uc *UseCase) Wait() { uc.wg.Wait() }

// Execute valida los enums y actualiza (o crea) las preferencias.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) error {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	// Validar enums
	if cmd.PreferredClass != nil {
		if !isValidCabinClass(*cmd.PreferredClass) {
			return domain.ErrInvalidPreferredClass
		}
	}
	if cmd.SeatPreference != nil {
		if !isValidSeatPreference(*cmd.SeatPreference) {
			return domain.ErrInvalidSeatPreference
		}
	}
	if cmd.MaxLayoverDuration != nil && *cmd.MaxLayoverDuration < 0 {
		return domain.ErrInvalidMaxLayover
	}

	// Intentar obtener prefs existentes
	existing, err := uc.travelPrefsRepo.GetByUserID(ctx, userID)
	if err != nil && !errors.Is(err, domain.ErrProfileNotFound) {
		return fmt.Errorf("get travel preferences: %w", err)
	}

	if existing == nil {
		// Crear nuevas preferencias
		prefs := domain.NewTravelPreferences(userID)
		applyCommand(prefs, cmd)
		if err := uc.travelPrefsRepo.Create(ctx, prefs); err != nil {
			return fmt.Errorf("create travel preferences: %w", err)
		}
		uc.publishEvent(ctx, userID)
		return nil
	}

	// Actualizar existentes
	applyCommand(existing, cmd)
	if err := uc.travelPrefsRepo.Update(ctx, existing); err != nil {
		return fmt.Errorf("update travel preferences: %w", err)
	}

	uc.publishEvent(ctx, userID)
	return nil
}

// applyCommand aplica los campos no-nil del comando a las preferencias.
func applyCommand(tp *domain.TravelPreferences, cmd Command) {
	if cmd.PreferredClass != nil {
		tp.PreferredClass = domain.CabinClass(*cmd.PreferredClass)
	}
	if cmd.SeatPreference != nil {
		sp := domain.SeatPreference(*cmd.SeatPreference)
		tp.SeatPreference = &sp
	}
	if cmd.MealPreference != nil {
		tp.MealPreference = cmd.MealPreference
	}
	if cmd.SpecialAssistance != nil {
		tp.SpecialAssistance = cmd.SpecialAssistance
	}
	if cmd.PreferredAirlines != nil {
		airlines := make([]uuid.UUID, 0, len(cmd.PreferredAirlines))
		for _, a := range cmd.PreferredAirlines {
			if id, err := uuid.Parse(a); err == nil {
				airlines = append(airlines, id)
			}
		}
		tp.PreferredAirlines = airlines
	}
	if cmd.PreferredHotels != nil {
		tp.PreferredHotels = cmd.PreferredHotels
	}
	if cmd.AvoidLayovers != nil {
		tp.AvoidLayovers = *cmd.AvoidLayovers
	}
	if cmd.MaxLayoverDuration != nil {
		tp.MaxLayoverDuration = cmd.MaxLayoverDuration
	}
}

func isValidCabinClass(c string) bool {
	switch domain.CabinClass(c) {
	case domain.CabinClassEconomy, domain.CabinClassPremiumEconomy, domain.CabinClassBusiness, domain.CabinClassFirst:
		return true
	}
	return false
}

func isValidSeatPreference(s string) bool {
	switch domain.SeatPreference(s) {
	case domain.SeatWindow, domain.SeatAisle, domain.SeatMiddle, domain.SeatNoPreference:
		return true
	}
	return false
}

func (uc *UseCase) publishEvent(ctx context.Context, userID uuid.UUID) {
	if uc.eventPublisher == nil {
		return
	}
	uc.wg.Go(func() {
		bgCtx := context.WithoutCancel(ctx)
		_, err := uc.eventPublisher.Publish(bgCtx,
			eventbus.StreamName("user.travel_preferences.updated"),
			map[string]interface{}{
				"user_id": userID.String(),
			},
		)
		if err != nil {
			slog.WarnContext(bgCtx, "publish travel preferences updated event failed",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()),
			)
		}
	})
}
