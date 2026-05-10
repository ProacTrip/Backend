package eventbus

// =============================================================================
// Constructores de eventos del dominio
// Cada función crea un Event con tipo, aggregate ID y payload
// =============================================================================

import (
	"time"
)

// NewUserRegisteredEvent crea un evento de usuario registrado.
// Incluye verification_token para que el consumer de notification pueda enviar el email.
// Los campos de entorno (languageCode, currencyCode, countryCode, timezoneName)
// son opcionales: se incluyen en el payload solo cuando no están vacíos.
// Esto asegura que eventos legacy (sin estos campos) sigan siendo válidos al deserializar.
func NewUserRegisteredEvent(userID, email, verificationToken, languageCode, currencyCode, countryCode, timezoneName string) Event {
	payload := map[string]interface{}{
		"user_id": userID,
		"email":   email,
	}

	// Incluir verification_token para el notification consumer
	if verificationToken != "" {
		payload["verification_token"] = verificationToken
	}

	// Env fields — solo incluidos cuando no están vacíos (omitempty equivalente en map)
	if languageCode != "" {
		payload["language_code"] = languageCode
	}
	if currencyCode != "" {
		payload["currency_code"] = currencyCode
	}
	if countryCode != "" {
		payload["country_code"] = countryCode
	}
	if timezoneName != "" {
		payload["timezone_name"] = timezoneName
	}

	return Event{
		EventType:   UserRegistered,
		AggregateID: userID,
		Timestamp:   time.Now().UnixMilli(),
		Payload:     payload,
	}
}

// NewTripCreatedEvent creates a trip_created domain event.
func NewTripCreatedEvent(tripID, userID string) Event {
	return Event{
		EventType:   TripCreated,
		AggregateID: tripID,
		Timestamp:   time.Now().UnixMilli(),
		Payload: map[string]interface{}{
			"trip_id": tripID,
			"user_id": userID,
		},
	}
}

// NewTripUpdatedEvent creates a trip_updated domain event.
func NewTripUpdatedEvent(tripID, userID string) Event {
	return Event{
		EventType:   TripUpdated,
		AggregateID: tripID,
		Timestamp:   time.Now().UnixMilli(),
		Payload: map[string]interface{}{
			"trip_id": tripID,
			"user_id": userID,
		},
	}
}

// NewTripDeletedEvent creates a trip_deleted domain event.
func NewTripDeletedEvent(tripID, userID string) Event {
	return Event{
		EventType:   TripDeleted,
		AggregateID: tripID,
		Timestamp:   time.Now().UnixMilli(),
		Payload: map[string]interface{}{
			"trip_id": tripID,
			"user_id": userID,
		},
	}
}
