// SSE streaming helpers para el handler de AI search.
//
// Proporciona funciones para escribir eventos Server-Sent Events (SSE)
// al http.ResponseWriter. Usado por el handler para enviar respuestas
// de discovery en tiempo real al frontend.
package ai_search

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
// streamDiscoveryResponse — lee un string y lo emite como chunks SSE
// =============================================================================

// streamDiscoveryResponse chunks a discovery AI response into SSE events.
// Splits the full AI response into word-chunked SSE events so the frontend
// can progressively render the response.
//
// Event types emitted:
//   - "status": {"status": "thinking"} — immediately, before first chunk
//   - "chunk": {"content": "partial text..."} — content delta
//   - "done": {"full_text": "..."} — final event with complete response
func streamDiscoveryResponse(w http.ResponseWriter, fullText string) error {
	// Emit initial "thinking" status
	if err := writeSSEEventJSON(w, "status", map[string]string{"status": "thinking"}); err != nil {
		return err
	}

	// Split text into word-based chunks for progressive rendering.
	// Simple approach: split on spaces, emit chunks of ~5 words each.
	words := strings.Fields(fullText)
	chunkSize := 5
	totalWords := len(words)

	for i := 0; i < totalWords; i += chunkSize {
		end := i + chunkSize
		if end > totalWords {
			end = totalWords
		}
		chunk := strings.Join(words[i:end], " ")
		if i+chunkSize < totalWords {
			chunk += " "
		}

		if err := writeSSEEventJSON(w, "chunk", map[string]string{"content": chunk}); err != nil {
			return err
		}
	}

	// Emit final "done" event
	return writeSSEEventJSON(w, "done", map[string]string{"full_text": fullText})
}
