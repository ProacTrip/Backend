// DTO de respuesta para GET /v1/user/profile.
// Estructura flat alineada con docs/USER_API.md.
package get_profile

import (
	"github.com/google/uuid"
)

// GetProfileResponse es la respuesta completa del endpoint (flat, sin wrapper).
type GetProfileResponse struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	Email         string    `json:"email"`
	FirstName     *string   `json:"first_name,omitzero"`
	LastName      *string   `json:"last_name,omitzero"`
	DateOfBirth   *string   `json:"date_of_birth,omitzero"`
	Gender        *string   `json:"gender,omitzero"`
	Nationality   *string   `json:"nationality,omitzero"`
	Phone         *string   `json:"phone,omitzero"`
	Bio           *string   `json:"bio,omitzero"`
	AvatarURL     *string   `json:"avatar_url,omitzero"`
	IsPublic      bool      `json:"is_public"`

	Location                LocationResponse                 `json:"location"`
	TravelPreferences       *TravelPreferencesResponse       `json:"travel_preferences,omitzero"`
	NotificationPreferences NotificationPreferencesResponse  `json:"notification_preferences,omitzero"`
}

// LocationResponse representa los datos de ubicación (mismo formato que /v1/environment).
type LocationResponse struct {
	Country     string  `json:"country,omitzero"`
	CountryCode string  `json:"country_code,omitzero"`
	City        string  `json:"city,omitzero"`
	State       string  `json:"state,omitzero"`
	Zipcode     string  `json:"zipcode,omitzero"`
	Timezone    string  `json:"timezone,omitzero"`
	Currency    string  `json:"currency,omitzero"`
	Language    string  `json:"language,omitzero"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// TravelPreferencesResponse representa las preferencias de viaje.
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

// NotificationPreferencesResponse es un mapa keyed por tipo de notificación.
// Keys: "booking_confirm", "price_alert", "travel_reminder", "promo_offer", etc.
type NotificationPreferencesResponse map[string]ChannelPreferences

// ChannelPreferences agrupa canales habilitados para un tipo de notificación.
type ChannelPreferences struct {
	Email     bool `json:"email"`
	SMS       bool `json:"sms"`
	Websocket bool `json:"websocket"`
}
