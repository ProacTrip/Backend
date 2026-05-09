// Consumer de validación de documentos.
// Consume eventos del stream {events}:doc:validate en Dragonfly.
// Descarga metadatos + archivo raw de R2, cross-valida MIME y estructura,
// y produce al siguiente paso del pipeline.
package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/filetype"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Constantes
// =============================================================================

const (
	docValidateStream = "{events}:doc:validate"
	docValidateGroup  = "doc-validate-group"
	docSanitizeStream = "{events}:doc:sanitize"
)

// =============================================================================
// Ports
// =============================================================================

// ValidatorR2Client es el puerto para interactuar con R2 desde el validador.
type ValidatorR2Client interface {
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	HeadContentType(ctx context.Context, bucket, key string) (string, error)
}

// =============================================================================
// ValidatorWorker — Consumer de Dragonfly Streams para validación
// =============================================================================

// ValidatorWorker consume el stream {events}:doc:validate, descarga archivos raw
// de R2, cross-valida MIME y estructura, y produce al stream doc:sanitize.
type ValidatorWorker struct {
	rdb       *redis.Client
	docRepo   DocumentUpdater
	r2        ValidatorR2Client
	group     string
	consumer  string
	dlqStream string
	running   atomic.Bool
}

// DocumentUpdater es el puerto para actualizar documentos en PostgreSQL.
type DocumentUpdater interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	Update(ctx context.Context, doc *domain.UserDocument) error
}

// NewValidatorWorker crea un nuevo validador de documentos.
func NewValidatorWorker(rdb *redis.Client, docRepo DocumentUpdater, r2 ValidatorR2Client) *ValidatorWorker {
	return &ValidatorWorker{
		rdb:       rdb,
		docRepo:   docRepo,
		r2:        r2,
		group:     docValidateGroup,
		consumer:  fmt.Sprintf("doc-validator-%d", time.Now().UnixMilli()),
		dlqStream: DocDLQStream,
	}
}

// IsRunning reports whether the main consume goroutine is alive.
func (v *ValidatorWorker) IsRunning() bool { return v.running.Load() }

// Name returns a human-readable identifier for health check reporting.
func (v *ValidatorWorker) Name() string { return "doc-validator" }

// Run inicia el consumer en background. Retorna inmediatamente.
func (v *ValidatorWorker) Run(ctx context.Context) error {
	if err := eventbus.EnsureConsumerGroup(ctx, v.rdb, docValidateStream, v.group); err != nil {
		return fmt.Errorf("ensure consumer group %s: %w", v.group, err)
	}

	_ = eventbus.EnsureConsumerGroup(ctx, v.rdb, docSanitizeStream, docSanitizeGroup)

	v.running.Store(true)
	go func() {
		defer v.running.Store(false)
		v.consume(ctx)
	}()

	go v.rescueOrphans(ctx)

	slog.Info("doc validator worker started", "group", v.group, "consumer", v.consumer)
	return nil
}

// =============================================================================
// Worker loop
// =============================================================================

func (v *ValidatorWorker) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := v.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    v.group,
			Consumer: v.consumer,
			Streams:  []string{docValidateStream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			slog.Error("doc validator: xreadgroup error", "error", err)
			continue
		}

		for _, stream := range messages {
			for _, msg := range stream.Messages {
				v.processMessage(ctx, msg)
			}
		}
	}
}

// =============================================================================
// Procesamiento de mensajes
// =============================================================================

func (v *ValidatorWorker) processMessage(ctx context.Context, msg redis.XMessage) {
	docIDStr, ok := msg.Values["document_id"].(string)
	if !ok {
		slog.Error("doc validator: missing document_id in payload", "msg_id", msg.ID)
		_ = v.rdb.XAck(ctx, docValidateStream, v.group, msg.ID)
		return
	}

	storageKey, ok := msg.Values["storage_key"].(string)
	if !ok {
		slog.Error("doc validator: missing storage_key in payload", "msg_id", msg.ID)
		_ = v.rdb.XAck(ctx, docValidateStream, v.group, msg.ID)
		return
	}

	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		slog.Error("doc validator: invalid document_id", "document_id", docIDStr, "msg_id", msg.ID)
		_ = v.rdb.XAck(ctx, docValidateStream, v.group, msg.ID)
		return
	}

	// 1. Cargar documento de la DB
	doc, err := v.docRepo.GetByID(ctx, docID)
	if err != nil {
		if err == domain.ErrDocumentNotFound {
			slog.Error("doc validator: document not found in DB", "doc_id", docID, "msg_id", msg.ID)
			_ = v.rdb.XAck(ctx, docValidateStream, v.group, msg.ID)
			return
		}
		slog.Error("doc validator: get document error", "doc_id", docID, "error", err)
		return
	}

	// 2. Transition: queued → processing on first pick-up
	if doc.OCRStatus == domain.OCRStatusQueued {
		doc.OCRStatus = domain.OCRStatusProcessing
		doc.UpdatedAt = time.Now()
		if err := v.docRepo.Update(ctx, doc); err != nil {
			slog.Error("doc validator: update status to processing failed", "doc_id", docID, "error", err)
			return
		}
	}

	detectedMime, _ := msg.Values["detected_mime_type"].(string)

	// 3. Validate MIME is accepted
	if !filetype.IsAccepted(detectedMime) {
		v.rejectDocument(ctx, doc, "MIME type no aceptado: "+detectedMime)
		_ = v.rdb.XAck(ctx, docValidateStream, v.group, msg.ID)
		v.publishSSEEvent(ctx, docID, "rejected", map[string]interface{}{
			"failure_reason": "invalid_mime_type",
			"detail":         "El tipo de archivo no es aceptado.",
		})
		return
	}

	// 4. Cross-validation — only if R2 client is available
	if v.r2 != nil {
		if rejection := v.crossValidate(ctx, storageKey, detectedMime, docID); rejection != "" {
			v.rejectDocument(ctx, doc, rejection)
			_ = v.rdb.XAck(ctx, docValidateStream, v.group, msg.ID)
			v.publishSSEEvent(ctx, docID, "rejected", map[string]interface{}{
				"failure_reason": "cross_validation_failed",
				"detail":         rejection,
			})
			return
		}
	}

	// 5. Marcar como validado y avanzar a sanitizing
	doc.OCRStatus = domain.OCRStatusSanitizing
	doc.UpdatedAt = time.Now()
	if detectedMime != "" {
		doc.DetectedMimeType = &detectedMime
	}

	if err := v.docRepo.Update(ctx, doc); err != nil {
		slog.Error("doc validator: update doc status failed", "doc_id", docID, "error", err)
		return
	}

	// 6. Publicar SSE
	v.publishSSEEvent(ctx, docID, "processing", map[string]interface{}{
		"sub_state": "validating",
		"message":   "Validación cruzada completada. Pasando a sanitización...",
	})

	// 7. Producir al siguiente stream: {events}:doc:sanitize
	payload := map[string]interface{}{
		"document_id":        docIDStr,
		"user_id":            msg.Values["user_id"],
		"storage_key":        storageKey,
		"file_name":          msg.Values["file_name"],
		"detected_mime_type": detectedMime,
		"timestamp":          fmt.Sprintf("%d", time.Now().UnixMilli()),
	}

	if _, err := v.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: docSanitizeStream,
		ID:     "*",
		Values: payload,
	}).Result(); err != nil {
		slog.Error("doc validator: produce to sanitize stream failed", "doc_id", docID, "error", err)
		return
	}

	// 8. XACK
	_ = v.rdb.XAck(ctx, docValidateStream, v.group, msg.ID)
}

// =============================================================================
// Cross-validation
// =============================================================================

// crossValidate verifica que el archivo en R2 coincida con el MIME detectado.
//
// MIME VALIDATION PRIORITY:
// Magic bytes (512) read in handler is the SOURCE OF TRUTH.
// Extension and R2 metadata MIME are CONSISTENCY CHECKS only.
// - If magic bytes says PDF but extension is .jpg → REJECT (suspicious mismatch)
// - If all three agree → PASS
//  1. Extension matches MIME (consistency check against magic bytes detection)
//  2. R2 metadata ContentType matches MIME (consistency check)
//  3. Polyglot detection: file is valid for ONE format only
//  4. PDF-specific: header, trailer, page object
//
// Returns empty string on success, or rejection reason.
func (v *ValidatorWorker) crossValidate(ctx context.Context, storageKey, detectedMime string, docID uuid.UUID) string {
	// 1. Validate file extension matches detected MIME
	ext := strings.ToLower(filepath.Ext(storageKey))
	expectedExt := strings.ToLower(filetype.ExtFromMime(detectedMime))
	if !extMatchesMime(ext, expectedExt) {
		return fmt.Sprintf("extension mismatch: storage key has '%s', MIME %s expects '%s'", ext, detectedMime, expectedExt)
	}

	// 2. Validate R2 metadata ContentType matches detected MIME
	r2ContentType, err := v.r2.HeadContentType(ctx, "proactrip-secure", storageKey)
	if err != nil {
		slog.Warn("doc validator: head content type failed", "doc_id", docID, "key", storageKey, "error", err)
	} else if r2ContentType != "" && r2ContentType != detectedMime {
		return fmt.Sprintf("R2 metadata MIME mismatch: stored as '%s', detected as '%s'", r2ContentType, detectedMime)
	}

	// 3. Download file for polyglot + PDF checks
	reader, err := v.r2.Download(ctx, "proactrip-secure", storageKey)
	if err != nil {
		slog.Error("doc validator: download for cross-validation failed", "doc_id", docID, "error", err)
		return "unable to download file for validation"
	}
	defer reader.Close()

	fileBytes, err := io.ReadAll(reader)
	if err != nil {
		slog.Error("doc validator: read file for cross-validation failed", "doc_id", docID, "error", err)
		return "unable to read file for validation"
	}

	// 4. Polyglot detection — file should match exactly ONE of the accepted types
	if isPolyglot(fileBytes, detectedMime) {
		return "polyglot file detected: file matches multiple format signatures"
	}

	// 5. PDF-specific validation
	if detectedMime == "application/pdf" {
		if reason := validatePDFStructure(fileBytes); reason != "" {
			return reason
		}
	}

	return ""
}

// extMatchesMime checks if the file extension is compatible with the MIME type.
func extMatchesMime(actual, expected string) bool {
	// Strip leading dot for comparison
	actual = strings.TrimPrefix(actual, ".")
	expected = strings.TrimPrefix(expected, ".")

	switch expected {
	case "jpg", "jpeg":
		return actual == "jpg" || actual == "jpeg"
	default:
		return actual == expected
	}
}

// isPolyglot checks if the file bytes match magic bytes for more than one accepted format.
// The detectedMime is the one that already matched — we check if any OTHER format also matches.
func isPolyglot(data []byte, detectedMime string) bool {
	if len(data) < 4 {
		return false
	}

	matches := 0
	// JPEG: FF D8 FF
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		matches++
	}
	// PNG: 89 50 4E 47
	if len(data) >= 4 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		matches++
	}
	// PDF: %PDF
	if len(data) >= 4 && data[0] == 0x25 && data[1] == 0x50 && data[2] == 0x44 && data[3] == 0x46 {
		matches++
	}

	return matches > 1
}

// validatePDFStructure validates PDF has proper header, trailer, and at least one page object.
func validatePDFStructure(data []byte) string {
	if len(data) < 1024 {
		return "PDF too small for validation"
	}

	// Check %PDF header in first 8 bytes
	header := data[:min(8, len(data))]
	if !bytes.Contains(header, []byte("%PDF-")) {
		return "PDF missing %PDF- header"
	}

	// Check %%EOF trailer in last 1024 bytes
	tailStart := max(0, len(data)-1024)
	tail := data[tailStart:]
	if !bytes.Contains(tail, []byte("%%EOF")) {
		return "PDF missing %%EOF trailer"
	}

	// Check at least 1 page object: /Type /Page or /Type/Page
	if !bytes.Contains(data, []byte("/Type /Page")) && !bytes.Contains(data, []byte("/Type/Page")) {
		return "PDF has no page objects (/Type /Page)"
	}

	return ""
}

func (v *ValidatorWorker) rejectDocument(ctx context.Context, doc *domain.UserDocument, reason string) {
	doc.MarkValidationFailed(reason)
	if err := v.docRepo.Update(ctx, doc); err != nil {
		slog.Error("doc validator: update doc rejected failed", "doc_id", doc.ID, "error", err)
	}
}

// =============================================================================
// SSE publishing
// =============================================================================

func (v *ValidatorWorker) publishSSEEvent(ctx context.Context, docID uuid.UUID, event string, data map[string]interface{}) {
	stream := fmt.Sprintf("{events}:doc:events:%s", docID.String())

	payload := map[string]interface{}{
		"event":     event,
		"timestamp": fmt.Sprintf("%d", time.Now().UnixMilli()),
	}
	for k, val := range data {
		payload[k] = val
	}

	if _, err := v.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     "*",
		Values: payload,
	}).Result(); err != nil {
		slog.Warn("doc validator: publish SSE event failed", "doc_id", docID, "event", event, "error", err)
	}
}

// =============================================================================
// Orphan rescue
// =============================================================================

func (v *ValidatorWorker) rescueOrphans(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		messages, err := eventbus.RescueOrphanedMessages(ctx, v.rdb, docValidateStream, v.group, 5*time.Minute)
		if err != nil {
			slog.Error("doc validator: rescue orphans error", "error", err)
			continue
		}

		for _, msg := range messages {
			slog.Info("doc validator: reclaiming orphan message", "msg_id", msg.ID)
			v.processMessage(ctx, msg)
		}
	}
}
