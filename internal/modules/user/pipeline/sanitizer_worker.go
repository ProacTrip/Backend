// Consumer de sanitización de documentos.
// Consume eventos del stream {events}:doc:sanitize en Dragonfly.
// Descarga el archivo raw de R2, sanitiza según MIME type,
// sube la versión limpia a R2 y elimina el original.
package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Constantes
// =============================================================================

const (
	docSanitizeGroup = "doc-sanitize-group"
	docOCRStream     = "{events}:doc:ocr"
)

// =============================================================================
// Ports
// =============================================================================

// SanitizerR2Client es el puerto completo para R2 desde el sanitizer.
type SanitizerR2Client interface {
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, bucket, key string) error
}

// =============================================================================
// SanitizerWorker — Consumer de Dragonfly Streams para sanitización
// =============================================================================

// SanitizerWorker consume el stream {events}:doc:sanitize, descarga archivos raw
// de R2, elimina metadatos/EXIF, y produce al stream doc:ocr.
type SanitizerWorker struct {
	rdb        *redis.Client
	r2         SanitizerR2Client
	docRepo    DocumentUpdater
	group      string
	consumer   string
	dlqStream  string
	running    atomic.Bool
	orphanDone chan struct{} // cerrado cuando rescueOrphans termina
}

// NewSanitizerWorker crea un nuevo sanitizer de documentos.
func NewSanitizerWorker(rdb *redis.Client, r2 SanitizerR2Client, docRepo DocumentUpdater) *SanitizerWorker {
	return &SanitizerWorker{
		rdb:       rdb,
		r2:        r2,
		docRepo:   docRepo,
		group:     docSanitizeGroup,
		consumer:  fmt.Sprintf("doc-sanitizer-%d", time.Now().UnixMilli()),
		dlqStream: DocDLQStream,
	}
}

// IsRunning indica si la goroutine principal de consumo O rescueOrphans está activa.
func (s *SanitizerWorker) IsRunning() bool {
	return s.running.Load() || !isClosed(s.orphanDone)
}

// Name devuelve un identificador legible.
func (s *SanitizerWorker) Name() string { return "doc-sanitizer" }

// OrphanDone expone el canal que se cierra cuando rescueOrphans termina.
func (s *SanitizerWorker) OrphanDone() <-chan struct{} { return s.orphanDone }

// Run inicia el consumer en background.
func (s *SanitizerWorker) Run(ctx context.Context) error {
	if err := eventbus.EnsureConsumerGroup(ctx, s.rdb, docSanitizeStream, s.group); err != nil {
		return fmt.Errorf("ensure consumer group %s: %w", s.group, err)
	}

	// Ensure OCR stream consumer group exists
	_ = eventbus.EnsureConsumerGroup(ctx, s.rdb, docOCRStream, docOCRGroup)

	s.running.Store(true)
	s.orphanDone = make(chan struct{})
	go func() {
		defer s.running.Store(false)
		s.consume(ctx)
	}()
	go func() {
		defer close(s.orphanDone)
		s.rescueOrphans(ctx)
	}()

	slog.Info("doc sanitizer worker started", "group", s.group, "consumer", s.consumer)
	return nil
}

// =============================================================================
// Worker loop
// =============================================================================

func (s *SanitizerWorker) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    s.group,
			Consumer: s.consumer,
			Streams:  []string{docSanitizeStream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			slog.Error("doc sanitizer: xreadgroup error", "error", err)
			continue
		}

		for _, stream := range messages {
			for _, msg := range stream.Messages {
				s.processMessage(ctx, msg)
			}
		}
	}
}

// =============================================================================
// Procesamiento de mensajes
// =============================================================================

func (s *SanitizerWorker) processMessage(ctx context.Context, msg redis.XMessage) {
	docIDStr, ok := msg.Values["document_id"].(string)
	if !ok {
		slog.Error("doc sanitizer: missing document_id in payload", "msg_id", msg.ID)
		_ = s.rdb.XAck(ctx, docSanitizeStream, s.group, msg.ID)
		return
	}

	storageKey, ok := msg.Values["storage_key"].(string)
	if !ok {
		slog.Error("doc sanitizer: missing storage_key in payload", "msg_id", msg.ID)
		_ = s.rdb.XAck(ctx, docSanitizeStream, s.group, msg.ID)
		return
	}

	mimeType, _ := msg.Values["detected_mime_type"].(string)

	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		slog.Error("doc sanitizer: invalid document_id", "document_id", docIDStr, "msg_id", msg.ID)
		_ = s.rdb.XAck(ctx, docSanitizeStream, s.group, msg.ID)
		return
	}

	// 1. Cargar documento de la DB
	doc, err := s.docRepo.GetByID(ctx, docID)
	if err != nil {
		if err == domain.ErrDocumentNotFound {
			_ = s.rdb.XAck(ctx, docSanitizeStream, s.group, msg.ID)
			return
		}
		slog.Error("doc sanitizer: get document error", "doc_id", docID, "error", err)
		return
	}

	// 2. Publicar SSE: sanitizing
	s.publishSSEEvent(ctx, docID, "processing", map[string]interface{}{
		"sub_state": "sanitizing",
		"message":   "Sanitizando archivo...",
	})

	// 3. Descargar raw de R2
	reader, err := s.r2.Download(ctx, "proactrip-secure", storageKey)
	if err != nil {
		slog.Error("doc sanitizer: download raw file failed", "doc_id", docID, "key", storageKey, "error", err)
		return // No XACK — reintentar
	}
	defer reader.Close()

	rawBytes, err := io.ReadAll(reader)
	if err != nil {
		slog.Error("doc sanitizer: read raw file failed", "doc_id", docID, "error", err)
		return
	}

	// 4. Sanitizar según MIME type
	cleanBytes, err := s.sanitize(rawBytes, mimeType)
	if err != nil {
		slog.Error("doc sanitizer: sanitize failed", "doc_id", docID, "mime", mimeType, "error", err)
		doc.FailureReason = new(fmt.Sprintf("sanitization failed: %v", err))
		_ = s.docRepo.Update(ctx, doc)
		_ = s.rdb.XAck(ctx, docSanitizeStream, s.group, msg.ID)
		return
	}

	// 5. Determinar extensión para processed key
	ext := extForMime(mimeType)

	// Obtener userID del documento
	userID := doc.UserID

	// Construir processed key: processed/{userID}/{docID}/clean{ext}
	processedKey := fmt.Sprintf("processed/%s/%s/clean%s", userID.String(), docID.String(), ext)

	// NOTE: raw/ file is NOT deleted here.
	// raw/ is retained for audit, debugging, and re-processing.
	// Solo el handler DELETE /v1/user/documents/:id elimina archivos raw/.
	//
	// 6. Subir archivo sanitizado a R2/processed/
	cleanContentType := mimeType
	if err := s.r2.Upload(ctx, "proactrip-secure", processedKey,
		bytes.NewReader(cleanBytes), int64(len(cleanBytes)), cleanContentType); err != nil {
		slog.Error("doc sanitizer: upload processed file failed", "doc_id", docID, "error", err)
		return
	}

	// 7. Actualizar DB
	doc.StorageKey = processedKey
	doc.OCRStatus = domain.OCRStatusOCRProcessing
	doc.DetectedSizeBytes = new(int64(len(cleanBytes)))
	doc.UpdatedAt = time.Now()
	if err := s.docRepo.Update(ctx, doc); err != nil {
		slog.Error("doc sanitizer: update doc failed", "doc_id", docID, "error", err)
		return
	}

	// 8. Publicar SSE: sanitizing completo → camino a OCR
	s.publishSSEEvent(ctx, docID, "processing", map[string]interface{}{
		"sub_state": "ocr_processing",
		"message":   "Enviando a OCR...",
	})

	// 9. Producir al stream {events}:doc:ocr
	ocrPayload := map[string]interface{}{
		"document_id":        docIDStr,
		"user_id":            msg.Values["user_id"],
		"storage_key":        processedKey,
		"file_name":          msg.Values["file_name"],
		"detected_mime_type": mimeType,
		"timestamp":          fmt.Sprintf("%d", time.Now().UnixMilli()),
	}

	if _, err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: docOCRStream,
		ID:     "*",
		Values: ocrPayload,
	}).Result(); err != nil {
		slog.Error("doc sanitizer: produce to OCR stream failed", "doc_id", docID, "error", err)
		return
	}

	// 10. XACK
	_ = s.rdb.XAck(ctx, docSanitizeStream, s.group, msg.ID)
}

// =============================================================================
// Sanitización por tipo MIME
// =============================================================================

// sanitize procesa los bytes raw según el MIME type para eliminar metadatos.
func (s *SanitizerWorker) sanitize(data []byte, mime string) ([]byte, error) {
	switch mime {
	case "image/jpeg":
		return sanitizeJPEG(data)
	case "image/png":
		return sanitizePNG(data)
	case "application/pdf":
		return sanitizePDF(data)
	default:
		return data, nil
	}
}

// sanitizeJPEG re-encodea la imagen JPEG para eliminar EXIF y metadatos.
func sanitizeJPEG(data []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, fmt.Errorf("re-encode JPEG: %w", err)
	}

	return buf.Bytes(), nil
}

// sanitizePNG re-encodea la imagen PNG para eliminar metadatos.
func sanitizePNG(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode PNG: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("re-encode PNG: %w", err)
	}

	return buf.Bytes(), nil
}

// sanitizePDF strips dangerous entries from PDF structure.
// Enfoque: analiza la estructura de objetos PDF, elimina entradas peligrosas, escribe PDF limpio.
// Elimina:
//   - JavaScript (/JS, /JavaScript entries)
//   - Embedded files (/EmbeddedFile, /EmbeddedFiles)
//   - OpenAction, AA (Additional Actions)
//   - Launch actions
//   - URI actions (phishing links)
//   - SubmitForm actions
func sanitizePDF(data []byte) ([]byte, error) {
	dangerousPatterns := []string{
		"/JS",
		"/JavaScript",
		"/OpenAction",
		"/AA",
		"/Launch",
		"/EmbeddedFile",
		"/EmbeddedFiles",
		"/URI",
		"/SubmitForm",
	}

	cleaned := data
	for _, pattern := range dangerousPatterns {
		cleaned = removePDFObject(cleaned, pattern)
	}
	return cleaned, nil
}

// removePDFObject elimina objetos PDF que contienen el patrón indicado.
// Busca el patrón en líneas y neutraliza las referencias peligrosas.
// Enfoque conservador: no reestructura el PDF completo pero sí neutraliza
// las referencias a acciones peligrosas.
func removePDFObject(data []byte, pattern string) []byte {
	lines := bytes.Split(data, []byte("\n"))
	var result [][]byte
	for _, line := range lines {
		if bytes.Contains(line, []byte(pattern)) {
			result = append(result, []byte("% sanitized by ProacTrip"))
			continue
		}
		result = append(result, line)
	}
	return bytes.Join(result, []byte("\n"))
}

// =============================================================================
// Helpers
// =============================================================================

// extForMime mapea un MIME type a su extensión de archivo.
func extForMime(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

// publishSSEEvent publica un evento SSE en el stream doc:events:{id}.
func (s *SanitizerWorker) publishSSEEvent(ctx context.Context, docID uuid.UUID, event string, data map[string]interface{}) {
	stream := fmt.Sprintf("{events}:doc:events:%s", docID.String())

	payload := map[string]interface{}{
		"event":     event,
		"timestamp": fmt.Sprintf("%d", time.Now().UnixMilli()),
	}
	for k, val := range data {
		payload[k] = val
	}

	if _, err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     "*",
		Values: payload,
	}).Result(); err != nil {
		slog.Warn("doc sanitizer: publish SSE event failed", "doc_id", docID, "event", event, "error", err)
	}
}

// =============================================================================
// Orphan rescue
// =============================================================================

func (s *SanitizerWorker) rescueOrphans(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		messages, err := eventbus.RescueOrphanedMessages(ctx, s.rdb, docSanitizeStream, s.group, 5*time.Minute)
		if err != nil {
			slog.Error("doc sanitizer: rescue orphans error", "error", err)
			continue
		}

		for _, msg := range messages {
			slog.Info("doc sanitizer: reclaiming orphan message", "msg_id", msg.ID)
			s.processMessage(ctx, msg)
		}
	}
}
