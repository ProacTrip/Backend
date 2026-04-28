package eventbus

// =============================================================================
// Constructores de eventos del dominio
// Cada función crea un Event con tipo, aggregate ID y payload
// =============================================================================

import (
	"time"
)

// NewUserRegisteredEvent crea un evento de usuario registrado
// Incluye verification_token para que el consumer de notification pueda enviar el email
func NewUserRegisteredEvent(userID, email, verificationToken string) Event {
	payload := map[string]interface{}{
		"user_id": userID,
		"email":   email,
	}

	// Incluir verification_token para el notification consumer
	if verificationToken != "" {
		payload["verification_token"] = verificationToken
	}

	return Event{
		EventType:   UserRegistered,
		AggregateID: userID,
		Timestamp:   time.Now().UnixMilli(),
		Payload:     payload,
	}
}

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
