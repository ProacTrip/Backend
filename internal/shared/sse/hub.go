// Hub seguro para hilos (thread-safe) para canales SSE por usuario.
// Todos los módulos publican eventos en el hub; los clientes SSE conectados los reciben.
package sse

import (
	"sync"

	"github.com/google/uuid"
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
	h.mu.RLock()
	userChannels := h.channels[userID]
	h.mu.RUnlock()

	if userChannels == nil {
		return
	}

	for ch := range userChannels {
		select {
		case ch <- event:
		default:
			// Descarta el evento para consumidores lentos — no bloquea al publicador.
		}
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
