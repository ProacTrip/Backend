// Puente SSE cross-instance vía Dragonfly Pub/Sub.
// Cada instancia publica eventos en {sse}:events y se subscribe a ese mismo canal
// para recibir eventos publicados por otras instancias, haciendo fan-out hacia
// el Hub local vía publishLocal (nunca PublishAndBridge — previene loop infinito).
package sse

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// BridgeMessage es el payload JSON serializado en el canal Dragonfly Pub/Sub.
// Deserializado por el bridge subscriber de cada instancia para reconstruir el
// evento y entregarlo al Hub local.
type BridgeMessage struct {
	UserID    string `json:"user_id"`
	EventType string `json:"event_type"`
	EventData any    `json:"event_data"`
}

// Bridge escucha el canal Dragonfly Pub/Sub {sse}:events y reenvía los eventos
// al Hub local. Es un singleton que corre durante toda la vida del proceso.
// Si rdb es nil, el bridge es un no-op (Start retorna inmediatamente).
type Bridge struct {
	hub      *Hub
	rdb      *redis.Client
	mu       sync.Mutex
	running  bool
	shutdown chan struct{}
}

// NewBridge crea un nuevo Bridge. No inicia la goroutine — llamar a Start().
func NewBridge(hub *Hub, rdb *redis.Client) *Bridge {
	return &Bridge{
		hub:      hub,
		rdb:      rdb,
		shutdown: make(chan struct{}),
	}
}

// Start inicia la goroutine del subscriber en background.
// Idempotente: llamadas subsecuentes son no-ops. Nil rdb → retorna sin hacer nada.
func (b *Bridge) Start(ctx context.Context) {
	if b.rdb == nil {
		return
	}

	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.running = true
	b.mu.Unlock()

	go b.subscriptionLoop(ctx)
}

// subscriptionLoop se subscribe a {sse}:events en Dragonfly Pub/Sub y reenvía
// mensajes al Hub local. Usa exponential backoff en la reconexión.
// Para en: ctx.Done() o shutdown channel.
func (b *Bridge) subscriptionLoop(ctx context.Context) {
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
	)
	backoff := initialBackoff

	for {
		select {
		case <-b.shutdown:
			return
		case <-ctx.Done():
			return
		default:
		}

		pubsub := b.rdb.Subscribe(ctx, "{sse}:events")
		ch := pubsub.Channel()

		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					goto reconnect
				}
				b.handleMessage(msg.Payload)

			case <-b.shutdown:
				pubsub.Close()
				return

			case <-ctx.Done():
				pubsub.Close()
				return
			}
		}

	reconnect:
		pubsub.Close()

		select {
		case <-time.After(backoff):
		case <-b.shutdown:
			return
		case <-ctx.Done():
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// handleMessage deserializa un mensaje Pub/Sub y lo entrega al Hub local.
// Mensajes malformados se dropean con log WARN. UUID inválido → dropeado.
// Usa publishLocal (no PublishAndBridge) para prevenir loops infinitos
// (SSE-BRIDGE-003).
func (b *Bridge) handleMessage(payload string) {
	var bm BridgeMessage
	if err := json.Unmarshal([]byte(payload), &bm); err != nil {
		slog.Warn("sse: bridge failed to unmarshal message",
			slog.String("error", err.Error()),
		)
		return
	}

	userID, err := uuid.Parse(bm.UserID)
	if err != nil {
		slog.Warn("sse: bridge received invalid user_id",
			slog.String("user_id", bm.UserID),
			slog.String("error", err.Error()),
		)
		return
	}

	b.hub.publishLocal(userID, Event{
		Type: bm.EventType,
		Data: bm.EventData,
	})
}
