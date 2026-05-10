package eventbus

// =============================================================================
// Publicador de eventos usando Redis Streams
// Publica a streams con hashtag para distribución en shards
// =============================================================================

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/redis/go-redis/v9"
)

const eventsHashtag = "{events}"

type EventBus struct {
	rdb *redis.Client
}

func NewEventBus(rdb *redis.Client) *EventBus {
	return &EventBus{rdb: rdb}
}

// Publish writes an event to a Dragonfly Stream via XADD.
// The payload is copied before adding a timestamp to avoid mutating the caller's map.
// The stream name should already include the hashtag prefix (e.g. "{events}:auth.user.registered").
func (e *EventBus) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	// Copiar payload para evitar mutar el mapa del caller (C5).
	// Agregamos el timestamp a la copia, no al original.
	// Si el caller ya seteó "timestamp", lo respetamos — no lo sobrescribimos.
	copied := make(map[string]any, len(payload)+1)
	maps.Copy(copied, payload)
	if _, hasTs := copied["timestamp"]; !hasTs {
		copied["timestamp"] = fmt.Sprintf("%d", time.Now().UnixMilli())
	}

	// IMPORTANTE: No agregar hashtag aquí - el caller debe pasar el stream name completo
	// con hashtag incluido. Esto evita el doble hashtag: {events}:{events}:stream
	// El patrón correcto según Dragonfly 1.38 skill es:
	//   stream := eventbus.StreamName("auth.user.registered") // → {events}:auth.user.registered
	//   eventBus.Publish(ctx, stream, payload) // → usa stream directamente

	id, err := e.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     "*",
		Values: copied,
	}).Result()
	return id, err
}

func StreamName(stream string) string {
	return fmt.Sprintf("%s:%s", eventsHashtag, stream)
}
