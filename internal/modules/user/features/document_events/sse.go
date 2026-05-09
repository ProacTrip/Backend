// Handler SSE para GET /v1/user/documents/:document_id/events.
// Server-Sent Events para seguimiento en tiempo real del pipeline de documentos.
// Late-connection: emite un evento sintético con el estado actual desde Dragonfly cache.
package document_events

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
)

// DocRepo es el puerto para obtener metadata del documento.
type DocRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

// Handler procesa GET /v1/user/documents/:document_id/events (SSE).
type Handler struct {
	docRepo   DocRepo
	dragonfly *redis.Client
}

// NewHandler crea una nueva instancia del handler SSE.
func NewHandler(docRepo DocRepo, dragonfly *redis.Client) *Handler {
	return &Handler{docRepo: docRepo, dragonfly: dragonfly}
}

// Handle establece la conexión SSE y streamea eventos del pipeline.
func (h *Handler) Handle(c *echo.Context) error {
	// Extraer user claims
	claims, err := echo.ContextGet[*token.AccessClaims](c, "user_claims")
	if err != nil {
		return httperr.MapError(c, err)
	}

	docID, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	// Verificar existencia y ownership
	doc, err := h.docRepo.GetByID(c.Request().Context(), docID)
	if err != nil {
		return httperr.MapError(c, err)
	}
	if doc.UserID != claims.UserID {
		return httperr.MapError(c, domain.ErrDocumentNotFound)
	}

	// Setear headers SSE
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	// Obtener flusher para escritura en tiempo real
	w := c.Response()
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported by client")
	}

	ctx := c.Request().Context()
	streamKey := fmt.Sprintf("{events}:doc:events:%s", docID.String())
	statusCacheKey := fmt.Sprintf("doc:status:%s", docID.String())

	// Late-connection: emitir evento sintético con estado actual
	h.emitSyntheticEvent(ctx, w, flusher, doc, statusCacheKey)

	// Leer eventos históricos del stream (desde el inicio)
	lastID := "0-0"

	// Enviar eventos históricos primero
	h.sendHistoricalEvents(ctx, w, flusher, streamKey, &lastID)

	// Bloquear esperando nuevos eventos
	keepaliveTicker := time.NewTicker(30 * time.Second)
	defer keepaliveTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-keepaliveTicker.C:
			// Keepalive: comentario SSE
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return nil
			}
			flusher.Flush()
		}

		// Leer nuevos eventos del stream (bloqueante hasta 25s)
		streams, err := h.dragonfly.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey, lastID},
			Count:   10,
			Block:   25 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			slog.Warn("SSE: xread error", "doc_id", docID, "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, s := range streams {
			for _, msg := range s.Messages {
				// Actualizar lastID para seguir desde este punto
				lastID = msg.ID

				// Formatear y enviar evento SSE
				h.writeSSEEvent(w, flusher, msg.Values)
				flusher.Flush()

				// Si es un evento terminal, cerrar
				if event, ok := msg.Values["event"].(string); ok {
					if event == "completed" || event == "rejected" || event == "failed" {
						return nil
					}
				}
			}
		}
	}
}

// emitSyntheticEvent emite un evento sintético para late-connections.
// Consulta la cache doc:status:{id} en Dragonfly para conocer el estado actual.
func (h *Handler) emitSyntheticEvent(ctx context.Context, w io.Writer, flusher http.Flusher, doc *domain.UserDocument, cacheKey string) {
	// Intentar cache primero
	cached, err := h.dragonfly.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var status map[string]interface{}
		if err := json.Unmarshal([]byte(cached), &status); err == nil {
			status["event"] = "processing"
			status["synthetic"] = true
			h.writeSSEEvent(w, flusher, status)
			flusher.Flush()
			return
		}
	}

	// Fallback: usar estado del documento en DB
	event := "processing"
	data := map[string]interface{}{
		"status":    string(doc.OCRStatus),
		"synthetic": true,
	}

	switch doc.OCRStatus {
	case domain.OCRStatusQueued:
		data["message"] = "Documento recibido. Esperando validación..."
	case domain.OCRStatusCompleted:
		event = "completed"
		if doc.DocumentType != nil {
			data["document_type"] = *doc.DocumentType
		}
		if doc.OCRConfidence != nil {
			data["ocr_confidence"] = *doc.OCRConfidence
		}
		data["message"] = "Documento procesado exitosamente."
	case domain.OCRStatusRejected:
		event = "rejected"
		if doc.FailureReason != nil {
			data["failure_reason"] = *doc.FailureReason
		}
		data["detail"] = "El archivo no contiene un documento reconocible."
	case domain.OCRStatusFailed:
		event = "failed"
		if doc.FailureReason != nil {
			data["failure_reason"] = *doc.FailureReason
		}
		data["detail"] = "Error técnico durante el procesamiento."
	case domain.OCRStatusValidating, domain.OCRStatusSanitizing, domain.OCRStatusOCRProcessing:
		data["sub_state"] = string(doc.OCRStatus)
		data["message"] = "Procesando documento..."
	}

	data["event"] = event
	h.writeSSEEvent(w, flusher, data)
	flusher.Flush()
}

// sendHistoricalEvents envía todos los eventos históricos del stream.
func (h *Handler) sendHistoricalEvents(ctx context.Context, w io.Writer, flusher http.Flusher, streamKey string, lastID *string) {
	streams, err := h.dragonfly.XRead(ctx, &redis.XReadArgs{
		Streams: []string{streamKey, "0-0"},
		Count:   100,
		Block:   0,
	}).Result()

	if err != nil || len(streams) == 0 {
		return
	}

	for _, s := range streams {
		for _, msg := range s.Messages {
			*lastID = msg.ID
			h.writeSSEEvent(w, flusher, msg.Values)
			flusher.Flush()
		}
	}
}

// writeSSEEvent escribe un evento SSE formateado en el writer.
func (h *Handler) writeSSEEvent(w io.Writer, flusher http.Flusher, data map[string]interface{}) {
	eventType, _ := data["event"].(string)
	if eventType == "" {
		eventType = "message"
	}

	// Escribir tipo de evento
	io.WriteString(w, fmt.Sprintf("event: %s\n", eventType))

	// Escribir datos como JSON
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		slog.Warn("SSE: marshal event data failed", "error", err)
		jsonBytes = []byte("{}")
	}
	io.WriteString(w, fmt.Sprintf("data: %s\n\n", string(jsonBytes)))
}
