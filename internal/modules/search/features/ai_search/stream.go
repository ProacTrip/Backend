// SSE streaming helpers para el handler de AI search.
//
// Proporciona funciones para escribir eventos Server-Sent Events (SSE)
// al http.ResponseWriter. Usado por Tool Calling y el handler para enviar
// respuestas en tiempo real al frontend.
//
// Event types (payloads match JSON objects sent over SSE):
//   - "chunk": {"content": "partial text..."} — AI text delta
//   - "search": {"destination": "...", "type": "hotels|flights", "data": {...}} — search results
//   - "filters": {"available": {...}, "active": {...}} — filter options
//   - "status": {"status": "thinking"} — initial status
//   - "done": {"conversation_id": "...", "turn_count": N} — stream complete
//   - "error": {"error": "message"} — error event
package ai_search

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	envDomain "github.com/ProacTrip/Backend/internal/modules/environment/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// writeSSEEvent — escribe un evento SSE al ResponseWriter
// =============================================================================

// writeSSEEvent writes a named SSE event with a string data payload.
// Format: "event: {event}\ndata: {data}\n\n"
func writeSSEEvent(w http.ResponseWriter, event, data string) error {
	_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// writeSSEEventJSON escribe un evento SSE con payload JSON.
func writeSSEEventJSON(w http.ResponseWriter, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		slog.Error("writeSSEEventJSON: marshal failed", "error", err)
		return err
	}
	return writeSSEEvent(w, event, string(payload))
}



// =============================================================================
// Public SSE event writers
// =============================================================================

// WriteChunkEvent writes an SSE "chunk" event with a text delta.
func WriteChunkEvent(w http.ResponseWriter, content string) error {
	return writeSSEEventJSON(w, "chunk", map[string]string{"content": content})
}

// WriteSearchEvent writes an SSE "search" event with structured search results.
// destination is a human-readable location (e.g., "Barcelona, España" or "MAD→BCN").
// searchType is "hotels" or "flights".
// data is the search response payload.
func WriteSearchEvent(w http.ResponseWriter, destination, searchType string, data interface{}) error {
	return writeSSEEventJSON(w, "search", map[string]interface{}{
		"destination": destination,
		"type":        searchType,
		"data":        data,
	})
}

// WriteFiltersEvent writes an SSE "filters" event with available and active filters.
func WriteFiltersEvent(w http.ResponseWriter, available, active interface{}) error {
	return writeSSEEventJSON(w, "filters", map[string]interface{}{
		"available": available,
		"active":    active,
	})
}

// WriteDoneEvent writes an SSE "done" event marking the stream as complete.
func WriteDoneEvent(w http.ResponseWriter, conversationID string, turnCount int) error {
	return writeSSEEventJSON(w, "done", map[string]interface{}{
		"conversation_id": conversationID,
		"turn_count":      turnCount,
	})
}

// WriteErrorEvent writes an SSE "error" event.
func WriteErrorEvent(w http.ResponseWriter, message string) error {
	return writeSSEEventJSON(w, "error", map[string]string{"error": message})
}

// WriteWeatherEvent writes an SSE "weather" event with destination weather data.
// destination is a human-readable location name (e.g., "Barcelona, España").
// weather may be nil (graceful fallback: provider unavailable).
// Format: {"destination":"...","weather":{...}} or {"destination":"...","weather":null}
func WriteWeatherEvent(w http.ResponseWriter, destination string, weather *envDomain.WeatherData) error {
	return writeSSEEventJSON(w, "weather", map[string]interface{}{
		"destination": destination,
		"weather":     weather,
	})
}

// WriteMedicalAlertsEvent writes an SSE "alert" event with medical/travel alerts.
// The frontend displays these in a separate popup window.
func WriteMedicalAlertsEvent(w http.ResponseWriter, alerts []domain.MedicalAlert) error {
	if alerts == nil {
		alerts = []domain.MedicalAlert{}
	}
	return writeSSEEventJSON(w, "alert", map[string]interface{}{"alerts": alerts})
}
