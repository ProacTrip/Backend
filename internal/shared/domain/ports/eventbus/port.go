package eventbus

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Bus combina Publisher y Subscriber
type Bus interface {
	Publisher
	Subscriber
	HealthCheck(ctx context.Context) error
}

// Publisher define el contrato para publicar eventos al bus.
type Publisher interface {
	Publish(ctx context.Context, topic string, message *Message) error
	Close() error
}

// Subscriber define el contrato para suscribirse y consumir eventos.
type Subscriber interface {
	// Subscribe crea una suscripción de fan-out general.
	Subscribe(ctx context.Context, topic string) (Subscription, error)

	// SubscribeWithGroup crea una suscripción balanceada para un Consumer Group.
	SubscribeWithGroup(ctx context.Context, topic, group string) (Subscription, error)

	Close() error // Cierra el cliente global de suscripciones
}

type Subscription interface {
	// Receive bloquea hasta recibir un mensaje.
	Receive(ctx context.Context) (*Message, error)

	// Acknowledge confirma el mensaje.
	// ¡OJO! Ya no necesita recibir el topic ni el group, porque esta
	// interfaz ya nació atada a ellos cuando se llamó a SubscribeWithGroup.
	Acknowledge(ctx context.Context, messageIDs ...string) error

	// Close cierra esta suscripción individual, liberando el worker.
	Close() error
}

// Message representa un evento estándar transitando por el bus.
type Message struct {
	UUID      string    // ID único (v7) para garantizar idempotencia
	Payload   []byte    // Contenido serializado (JSON, Protobuf, etc.)
	Metadata  Metadata  // Cabeceras adicionales (TraceID, Origen)
	Timestamp time.Time // Fecha exacta de la ocurrencia del evento
}

// Metadata es un mapa de clave-valor para enriquecer el mensaje sin tocar el Payload.
type Metadata map[string]string

func (m Metadata) Get(key string) string {
	if m == nil {
		return ""
	}
	return m[key]
}

// Set añade o actualiza un valor en la metadata de forma segura.
func (m Metadata) Set(key, value string) {
	if m == nil {
		// En Go, no se puede inicializar el mapa base de un alias directamente
		// si se llama como método sobre un nil map. Esto requeriría un puntero.
		// Asumimos que Metadata se inicializa vía NewMetadata.
		return
	}
	m[key] = value
}

// Helpers de Metadata para convenciones estándar
func (m Metadata) EventType() string   { return m.Get("event_type") }
func (m Metadata) AggregateID() string { return m.Get("aggregate_id") }
func (m Metadata) Source() string      { return m.Get("source") }

// NewMessage crea un evento inmutable y listo para ser publicado.
func NewMessage(payload []byte, eventType, aggregateID, source string) *Message {
	return &Message{
		UUID:      generateUUID(),
		Payload:   payload,
		Metadata:  NewMetadata(eventType, aggregateID, source),
		Timestamp: time.Now().UTC(),
	}
}

// NewMetadata crea las cabeceras base requeridas.
func NewMetadata(eventType, aggregateID, source string) Metadata {
	return Metadata{
		"event_type":   eventType,
		"aggregate_id": aggregateID,
		"source":       source,
		"version":      "1.0",
	}
}

func generateUUID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic("eventbus: failed to generate UUID v7: " + err.Error())
	}
	return id.String()
}

// BusError representa errores específicos del bus
type BusError string

func (e BusError) Error() string { return string(e) }

// Errores comunes del bus
const (
	ErrPublisherClosed    BusError = "eventbus: publisher connection closed"
	ErrSubscriberClosed   BusError = "eventbus: subscriber connection closed"
	ErrSubscriptionClosed BusError = "eventbus: active subscription closed"
	ErrTopicNotFound      BusError = "eventbus: topic or stream not found"
	ErrTimeout            BusError = "eventbus: operation timed out"
	ErrInvalidMessage     BusError = "eventbus: invalid or malformed message"
	ErrNoNewMessages      BusError = "eventbus: no new messages available in stream"
)
