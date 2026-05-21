// Caso de uso: Obtener preferencias de viaje del usuario.
package get_travel_preferences

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// TravelPrefsRepo permite obtener las preferencias de viaje del usuario.
type TravelPrefsRepo interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	TravelPrefsRepo TravelPrefsRepo
}

// UseCase implementa la consulta de preferencias de viaje.
type UseCase struct {
	travelPrefsRepo TravelPrefsRepo
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		travelPrefsRepo: deps.TravelPrefsRepo,
	}
}

// Execute obtiene las preferencias de viaje del usuario.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*TravelPreferencesResponse, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	tp, err := uc.travelPrefsRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := &TravelPreferencesResponse{
		PreferredClass:     string(tp.PreferredClass),
		AvoidLayovers:      tp.AvoidLayovers,
		MaxLayoverDuration: tp.MaxLayoverDuration,
		MealPreference:     tp.MealPreference,
		SpecialAssistance:  tp.SpecialAssistance,
		PreferredHotels:    tp.PreferredHotels,
	}

	if tp.SeatPreference != nil {
		s := string(*tp.SeatPreference)
		resp.SeatPreference = &s
	}

	if len(tp.PreferredAirlines) > 0 {
		airlines := make([]string, len(tp.PreferredAirlines))
		for i, a := range tp.PreferredAirlines {
			airlines[i] = a.String()
		}
		resp.PreferredAirlines = airlines
	}

	return resp, nil
}
