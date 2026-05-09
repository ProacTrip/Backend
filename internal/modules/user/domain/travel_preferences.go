// Domain: Preferencias de viaje del usuario.
// Define clases de cabina, asiento y preferencias de viaje.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Enums
// =============================================================================

// CabinClass representa la clase de cabina preferida.
type CabinClass string

const (
	CabinClassEconomy        CabinClass = "economy"
	CabinClassPremiumEconomy CabinClass = "premium_economy"
	CabinClassBusiness       CabinClass = "business"
	CabinClassFirst          CabinClass = "first"
)

// SeatPreference representa la preferencia de asiento.
type SeatPreference string

const (
	SeatWindow       SeatPreference = "window"
	SeatAisle        SeatPreference = "aisle"
	SeatMiddle       SeatPreference = "middle"
	SeatNoPreference SeatPreference = "no_preference"
)

// =============================================================================
// TravelPreferences — Preferencias de viaje (1:1 con perfil)
// =============================================================================

// TravelPreferences representa las preferencias de viaje del usuario.
// Alineado con la migración user_travel_preferences.
type TravelPreferences struct {
	ID                 uuid.UUID      `json:"id"`
	UserID             uuid.UUID      `json:"user_id"`
	PreferredClass     CabinClass     `json:"preferred_class"`
	SeatPreference     *SeatPreference `json:"seat_preference,omitzero"`
	MealPreference     *string        `json:"meal_preference,omitzero"`
	SpecialAssistance  []string       `json:"special_assistance,omitzero"`
	PreferredAirlines  []uuid.UUID    `json:"preferred_airlines,omitzero"`
	PreferredHotels    []string       `json:"preferred_hotels,omitzero"`
	AvoidLayovers      bool           `json:"avoid_layovers"`
	MaxLayoverDuration *int           `json:"max_layover_duration,omitzero"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// NewTravelPreferences crea nuevas preferencias con valores por defecto.
func NewTravelPreferences(userID uuid.UUID) *TravelPreferences {
	now := time.Now()
	return &TravelPreferences{
		ID:             uuid.Must(uuid.NewV7()),
		UserID:         userID,
		PreferredClass: CabinClassEconomy,
		AvoidLayovers:  false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// SetClasses establece la clase de cabina y preferencia de asiento.
func (tp *TravelPreferences) SetClasses(class CabinClass, seat SeatPreference) {
	tp.PreferredClass = class
	tp.SeatPreference = &seat
	tp.UpdatedAt = time.Now()
}

// SetMealPreference establece la preferencia de comida.
func (tp *TravelPreferences) SetMealPreference(meal string) {
	tp.MealPreference = &meal
	tp.UpdatedAt = time.Now()
}

// SetLayoverPrefs establece preferencias de escala.
func (tp *TravelPreferences) SetLayoverPrefs(avoid bool, maxDuration *int) {
	tp.AvoidLayovers = avoid
	tp.MaxLayoverDuration = maxDuration
	tp.UpdatedAt = time.Now()
}
