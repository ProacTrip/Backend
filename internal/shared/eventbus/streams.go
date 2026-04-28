package eventbus

// =============================================================================
// Publicador de eventos usando Redis Streams
// Publica a streams con hashtag para distribución en shards
// =============================================================================

import (
	"context"
	"fmt"
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

func (e *EventBus) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	payload["timestamp"] = fmt.Sprintf("%d", time.Now().UnixMilli())

	// IMPORTANTE: No agregar hashtag aquí - el caller debe pasar el stream name completo
	// con hashtag incluido. Esto evita el doble hashtag: {events}:{events}:stream
	// El patrón correcto según Dragonfly 1.38 skill es:
	//   stream := eventbus.StreamName("auth.user.registered") // → {events}:auth.user.registered
	//   eventBus.Publish(ctx, stream, payload) // → usa stream directamente

	id, err := e.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     "*",
		Values: payload,
	}).Result()
	return id, err
}

func StreamName(stream string) string {
	return fmt.Sprintf("%s:%s", eventsHashtag, stream)
}
