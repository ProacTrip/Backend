package eventbus

// =============================================================================
// Definición de tipos de eventos del dominio
// =============================================================================

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	UserRegistered      EventType = "user_registered"
	UserVerified        EventType = "user_verified"
	TripCreated         EventType = "trip_created"
	TripUpdated         EventType = "trip_updated"
	TripDeleted         EventType = "trip_deleted"
	ConversationSaved   EventType = "conversation_saved"
	AccountDisabled     EventType = "account_disabled"
	AccountEnabled      EventType = "account_enabled"
)

type Event struct {
	EventType   EventType              `json:"event_type"`
	AggregateID string                 `json:"aggregate_id"`
	Timestamp   int64                  `json:"timestamp"`
	Payload     map[string]interface{} `json:"payload"`
}

func (e *Event) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"event_type":   string(e.EventType),
		"aggregate_id": e.AggregateID,
		"timestamp":    e.Timestamp,
		"payload":      e.Payload,
	}
}

func EventFromMap(m map[string]interface{}) (*Event, error) {
	eventType, ok := m["event_type"].(string)
	if !ok {
		eventType = string(UserRegistered)
	}

	aggregateID, _ := m["aggregate_id"].(string)

	var ts int64
	switch v := m["timestamp"].(type) {
	case float64:
		ts = int64(v)
	case int64:
		ts = v
	default:
		ts = time.Now().UnixMilli()
	}

	payload, _ := m["payload"].(map[string]interface{})
	if payload == nil {
		payload = make(map[string]interface{})
		for k, v := range m {
			if k != "event_type" && k != "aggregate_id" && k != "timestamp" && k != "payload" {
				payload[k] = v
			}
		}
	}

	return &Event{
		EventType:   EventType(eventType),
		AggregateID: aggregateID,
		Timestamp:   ts,
		Payload:     payload,
	}, nil
}

func (e *Event) JSON() ([]byte, error) {
	return json.Marshal(e)
}

func EventFromJSON(data []byte) (*Event, error) {
	var e Event
	err := json.Unmarshal(data, &e)
	return &e, err
}
