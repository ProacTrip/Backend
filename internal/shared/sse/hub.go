// Hub seguro para hilos (thread-safe) para canales SSE por usuario.
// Todos los módulos publican eventos en el hub; los clientes SSE conectados los reciben.
package sse

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Hub gestiona canales por usuario para la entrega de eventos en tiempo real.
// Seguro para hilos (thread-safe) mediante sync.RWMutex.
type Hub struct {
	mu       sync.RWMutex
	channels map[uuid.UUID]map[chan Event]struct{}
}

// NewHub devuelve un Hub listo para usar.
func NewHub() *Hub {
	return &Hub{
		channels: make(map[uuid.UUID]map[chan Event]struct{}),
	}
}

// Capacidad del canal para los buffers de eventos de usuario.
// Evita que los consumidores lentos bloqueen a los publicadores.
const channelBufferSize = 64

// Publish envía el evento a cada conexión activa para el usuario dado.
// Los consumidores lentos (buffer lleno) se descartan silenciosamente — un envío
// no bloqueante con un caso default asegura que los publicadores nunca se bloqueen.
func (h *Hub) Publish(userID uuid.UUID, event Event) {
	h.publishLocal(userID, event)
}

// publishLocal envía el evento a los canales del usuario sin pasar por el bridge.
// Es la ruta de entrega local (in-memory). La usa Publish (legacy local-only),
// PublishAndBridge (local + cross-instance), y el bridge subscriber (solo local,
// sin re-publicar a Pub/Sub como prevención de loop).
func (h *Hub) publishLocal(userID uuid.UUID, event Event) {
	h.mu.RLock()
	userChannels := h.channels[userID]
	h.mu.RUnlock()

	if userChannels == nil {
		return
	}

	for ch := range userChannels {
		select {
		case ch <- event:
			if event.Type != "" {
				slog.Debug("sse: event delivered", "user_id", userID, "type", event.Type)
			}
		default:
			slog.Warn("sse: event dropped (channel full)", "user_id", userID, "type", event.Type)
		}
	}
}

// PublishAndBridge entrega el evento localmente Y lo publica en el canal
// Dragonfly Pub/Sub {sse}:events para fan-out cross-instance.
// Nil rdb → solo entrega local (no-op del bridge).
// Error de PUBLISH → log WARN, la entrega local continúa.
func (h *Hub) PublishAndBridge(ctx context.Context, rdb *redis.Client, userID uuid.UUID, event Event) {
	h.publishLocal(userID, event)

	if rdb == nil {
		return
	}

	msg := BridgeMessage{
		UserID:    userID.String(),
		EventType: event.Type,
		EventData: event.Data,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		slog.Warn("sse: failed to marshal bridge message",
			slog.String("user_id", userID.String()),
			slog.String("event_type", event.Type),
			slog.String("error", err.Error()),
		)
		return
	}

	if err := rdb.Publish(ctx, "{sse}:events", payload).Err(); err != nil {
		slog.Warn("sse: bridge publish failed",
			slog.String("user_id", userID.String()),
			slog.String("event_type", event.Type),
			slog.String("error", err.Error()),
		)
	}
}

// Subscribe crea un canal con buffer y lo registra para el userID.
// El canal devuelto es cerrado por Unsubscribe (nunca por el llamador).
func (h *Hub) Subscribe(userID uuid.UUID) chan Event {
	ch := make(chan Event, channelBufferSize)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.channels[userID] == nil {
		h.channels[userID] = make(map[chan Event]struct{})
	}
	h.channels[userID][ch] = struct{}{}

	return ch
}

// Unsubscribe elimina el canal para el userID y lo cierra.
// Es seguro llamarlo múltiples veces — idempotente mediante búsqueda en el mapa.
func (h *Hub) Unsubscribe(userID uuid.UUID, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	userChannels, ok := h.channels[userID]
	if !ok {
		return
	}

	if _, exists := userChannels[ch]; exists {
		delete(userChannels, ch)
		close(ch)
	}

	// Recolecta de basura para entradas de usuario vacías.
	if len(userChannels) == 0 {
		delete(h.channels, userID)
	}
}
