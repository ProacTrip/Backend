// Comando para PATCH /v1/user/profile/travel-preferences.
package update_travel_prefs

import (
	"github.com/google/uuid"
)

// Command contiene los campos actualizables de preferencias de viaje.
// Todos opcionales: nil = no actualizar.
type Command struct {
	UserID             string    `json:"-"`
	PreferredClass     *string   `json:"preferred_class,omitzero"`
	SeatPreference     *string   `json:"seat_preference,omitzero"`
	MealPreference     *string   `json:"meal_preference,omitzero"`
	SpecialAssistance  []string  `json:"special_assistance,omitzero"`
	PreferredAirlines  []string  `json:"preferred_airlines,omitzero"` // UUID strings
	PreferredHotels    []string  `json:"preferred_hotels,omitzero"`
	AvoidLayovers      *bool     `json:"avoid_layovers,omitzero"`
	MaxLayoverDuration *int      `json:"max_layover_duration,omitzero"`
}

func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	return nil
}
