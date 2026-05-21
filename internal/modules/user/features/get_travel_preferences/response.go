// DTO de respuesta para GET /v1/user/profile/travel-preferences.
// Alineado con docs/USER_API.md § Get Travel Preferences.
package get_travel_preferences

// TravelPreferencesResponse representa las preferencias de viaje del usuario.
type TravelPreferencesResponse struct {
	PreferredClass     string   `json:"preferred_class"`
	SeatPreference     *string  `json:"seat_preference,omitzero"`
	MealPreference     *string  `json:"meal_preference,omitzero"`
	SpecialAssistance  []string `json:"special_assistance,omitzero"`
	PreferredAirlines  []string `json:"preferred_airlines,omitzero"`
	PreferredHotels    []string `json:"preferred_hotels,omitzero"`
	AvoidLayovers      bool     `json:"avoid_layovers"`
	MaxLayoverDuration *int     `json:"max_layover_duration,omitzero"`
}
