// Consumer de OCR para documentos.
// Consume eventos del stream {events}:doc:ocr en Dragonfly.
// Descarga el archivo procesado de R2, ejecuta OCR vía DeepSeek,
// extrae datos estructurados, detecta conflictos médicos y
// actualiza el perfil médico automáticamente cuando no hay conflictos.
package pipeline

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/storage"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/ProacTrip/Backend/internal/shared/sse"
)

// =============================================================================
// Constantes
// =============================================================================

const (
	docOCRGroup = "doc-ocr-group"
)

// =============================================================================
// Ports
// =============================================================================

// OCRR2Client es el puerto para interactuar con R2 desde el OCR worker.
type OCRR2Client interface {
	GenerateDownloadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
	Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error
}

// OCRDocUpdater extiende DocumentUpdater con operaciones específicas de OCR.
type OCRDocUpdater interface {
	DocumentUpdater
}

// MedicalProfileManager gestiona el perfil médico durante el OCR.
type MedicalProfileManager interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error)
	Update(ctx context.Context, profile *domain.MedicalProfile) error
	Create(ctx context.Context, profile *domain.MedicalProfile) error
}

// MedicalPendingCreator crea actualizaciones médicas pendientes.
type MedicalPendingCreator interface {
	Create(ctx context.Context, update *domain.MedicalPendingUpdate) error
}

// =============================================================================
// OCRWorker — Consumer de Dragonfly Streams para OCR
// =============================================================================

// OCRWorker consume el stream {events}:doc:ocr, ejecuta OCR vía IA,
// extrae datos y los persiste en PostgreSQL.
type OCRWorker struct {
	rdb               *redis.Client
	r2                OCRR2Client
	ocrService        domain.OCRService
	docRepo           OCRDocUpdater
	medicalRepo       MedicalProfileManager
	pendingRepo       MedicalPendingCreator
	encryptionService domain.EncryptionService
	group             string
	consumer          string
	dlqStream         string
	running           atomic.Bool
	orphanDone        chan struct{} // cerrado cuando rescueOrphans termina
}

// NewOCRWorker crea un nuevo OCR worker.
func NewOCRWorker(
	rdb *redis.Client,
	r2 OCRR2Client,
	ocrService domain.OCRService,
	docRepo OCRDocUpdater,
	medicalRepo MedicalProfileManager,
	pendingRepo MedicalPendingCreator,
	encryptionService domain.EncryptionService,
) *OCRWorker {
	return &OCRWorker{
		rdb:               rdb,
		r2:                r2,
		ocrService:        ocrService,
		docRepo:           docRepo,
		medicalRepo:       medicalRepo,
		pendingRepo:       pendingRepo,
		encryptionService: encryptionService,
		group:             docOCRGroup,
		consumer:          fmt.Sprintf("doc-ocr-%d", time.Now().UnixMilli()),
		dlqStream:         DocDLQStream,
	}
}

// IsRunning indica si la goroutine principal de consumo O rescueOrphans está activa.
func (w *OCRWorker) IsRunning() bool {
	return w.running.Load() || !isClosed(w.orphanDone)
}

// Name devuelve un identificador legible.
func (w *OCRWorker) Name() string { return "doc-ocr" }

// OrphanDone expone el canal que se cierra cuando rescueOrphans termina.
func (w *OCRWorker) OrphanDone() <-chan struct{} { return w.orphanDone }

// Run inicia el consumer en background.
func (w *OCRWorker) Run(ctx context.Context) error {
	if err := eventbus.EnsureConsumerGroup(ctx, w.rdb, docOCRStream, w.group); err != nil {
		return fmt.Errorf("ensure consumer group %s: %w", w.group, err)
	}

	w.running.Store(true)
	w.orphanDone = make(chan struct{})
	go func() {
		defer w.running.Store(false)
		w.consume(ctx)
	}()
	go func() {
		defer close(w.orphanDone)
		w.rescueOrphans(ctx)
	}()

	slog.Info("doc OCR worker started", "group", w.group, "consumer", w.consumer)
	return nil
}

// =============================================================================
// Worker loop
// =============================================================================

func (w *OCRWorker) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    w.group,
			Consumer: w.consumer,
			Streams:  []string{docOCRStream, ">"},
			Count:    5, // OCR es costoso — procesar de a pocos
			Block:    5 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			slog.Error("doc OCR: xreadgroup error", "error", err)
			continue
		}

		for _, stream := range messages {
			for _, msg := range stream.Messages {
				w.processMessage(ctx, msg)
			}
		}
	}
}

// =============================================================================
// Procesamiento de mensajes
// =============================================================================

func (w *OCRWorker) processMessage(ctx context.Context, msg redis.XMessage) {
	docIDStr, ok := msg.Values["document_id"].(string)
	if !ok {
		slog.Error("doc OCR: missing document_id in payload", "msg_id", msg.ID)
		_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
		return
	}

	storageKey, ok := msg.Values["storage_key"].(string)
	if !ok {
		slog.Error("doc OCR: missing storage_key in payload", "msg_id", msg.ID)
		_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
		return
	}

	userIDStr, _ := msg.Values["user_id"].(string)
	_ = msg.Values["detected_mime_type"] // no longer needed — OCR uses presigned URL

	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		slog.Error("doc OCR: invalid document_id", "document_id", docIDStr, "msg_id", msg.ID)
		_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		slog.Error("doc OCR: invalid user_id", "user_id", userIDStr, "msg_id", msg.ID)
		_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
		return
	}

	// 1. Cargar documento de DB
	doc, err := w.docRepo.GetByID(ctx, docID)
	if err != nil {
		if err == domain.ErrDocumentNotFound {
			_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
			return
		}
		slog.Error("doc OCR: get document error", "doc_id", docID, "error", err)
		return
	}

	w.publishSSEEvent(ctx, doc.UserID, docID, "processing", map[string]interface{}{
		"sub_state": "ocr_processing",
		"message":   "Extrayendo datos del documento...",
	})

	// 2. Generar URL prefirmada de descarga desde R2 (TTL 5 minutos)
	downloadURL, err := w.r2.GenerateDownloadURL(ctx, storage.SecureBucket(), storageKey, 5*time.Minute)
	if err != nil {
		slog.Error("doc OCR: generate download URL failed", "doc_id", docID, "key", storageKey, "error", err)
		w.markFailed(ctx, doc, "file_download_error")
		_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
		return
	}

	// 3. Ejecutar OCR / AI extraction usando la URL prefirmada
	extracted, err := w.ocrService.ExtractFromDocument(ctx, downloadURL)
	if err != nil {
		slog.Error("doc OCR: extraction failed", "doc_id", docID, "error", err)
		w.markFailed(ctx, doc, fmt.Sprintf("ocr_extraction_error: %v", err))
		_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
		return
	}

	// 4. Guardar respuesta raw del OCR en R2 results/
	resultsKey := fmt.Sprintf("results/%s/%s/ocr_raw.json", userID.String(), docID.String())
	resultsJSON, _ := json.Marshal(extracted)
	if err := w.r2.Upload(ctx, storage.SecureBucket(), resultsKey,
		bytes.NewReader(resultsJSON), int64(len(resultsJSON)), "application/json"); err != nil {
		slog.Warn("doc OCR: save OCR results to R2 failed", "doc_id", docID, "error", err)
	}

	// 5. Verificar si es un documento de viaje reconocido
	if !extracted.IsTravelDocument() {
		w.markRejected(ctx, doc, "not_a_travel_document")
		w.publishSSEEvent(ctx, doc.UserID, docID, "rejected", map[string]interface{}{
			"failure_reason": "not_a_travel_document",
			"detail":         "El archivo no contiene un documento de viaje reconocible.",
		})
		w.updateDocStatusCache(ctx, docID, "rejected")
		_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
		return
	}

	// 6. Actualizar documento con datos extraídos
	extractedJSON, _ := json.Marshal(extracted)
	doc.ExtractedData = extractedJSON
	doc.DocumentType = &extracted.DocumentType
	doc.OCRConfidence = &extracted.OCRConfidence
	doc.OCRStatus = domain.OCRStatusCompleted

	if extracted.DocumentNumber != nil {
		doc.DocumentNumber = extracted.DocumentNumber
	}
	if extracted.IssuingCountry != nil {
		doc.IssuingCountry = extracted.IssuingCountry
	}
	if extracted.ExpiryDate != nil {
		expiry, err := time.Parse("2006-01-02", *extracted.ExpiryDate)
		if err == nil {
			doc.ValidUntil = &expiry
		}
	}
	if extracted.FullName != nil {
		fullName := *extracted.FullName
		// Solo guardar en metadata — no actualizar el perfil del usuario
		meta := map[string]string{"full_name": fullName}
		metaJSON, _ := json.Marshal(meta)
		doc.Metadata = metaJSON
	}

	doc.UpdatedAt = time.Now()

	// 7. Comparar y aplicar datos médicos si existen
	if extracted.MedicalFields != nil && len(extracted.MedicalFields) > 0 {
		w.compareAndApplyMedicalData(ctx, doc, extracted)
	}

	// 8. Persistir documento actualizado
	if err := w.docRepo.Update(ctx, doc); err != nil {
		slog.Error("doc OCR: update document failed", "doc_id", docID, "error", err)
		return
	}

	// 9. Cache en Dragonfly
	w.updateDocStatusCache(ctx, docID, "completed")

	// 10. Publicar SSE: completed
	w.publishSSEEvent(ctx, doc.UserID, docID, "completed", map[string]interface{}{
		"document_type":   extracted.DocumentType,
		"ocr_confidence":   extracted.OCRConfidence,
		"message":         "Documento procesado exitosamente.",
	})

	slog.Info("doc OCR: completed", "doc_id", docID, "type", extracted.DocumentType,
		"confidence", extracted.OCRConfidence)

	_ = w.rdb.XAck(ctx, docOCRStream, w.group, msg.ID)
}

// =============================================================================
// Manejo de datos médicos
// =============================================================================

// medicalFieldMap mapea nombres de campos OCR a campos del perfil médico.
var medicalFieldMap = map[string]string{
	"blood_type":        "blood_type",
	"allergies":         "allergies",
	"medications":       "medications",
	"conditions":        "conditions",
	"vaccinations":      "vaccinations",
	"emergency_contact": "emergency_contact",
	"insurance_info":    "insurance_info",
}

// compareAndApplyMedicalData compara los datos médicos extraídos con el perfil actual.
// - Si el campo está vacío en el perfil → auto-aplica con source="ocr:{doc_id}"
// - Si el campo tiene un valor diferente → crea MedicalPendingUpdate
func (w *OCRWorker) compareAndApplyMedicalData(ctx context.Context, doc *domain.UserDocument, extracted *domain.ExtractedData) {
	userID := doc.UserID
	docID := doc.ID
	source := domain.SourceToDetail(domain.MedicalSourceOCR)
	docIDStr := docID.String()
	source.DocumentID = &docIDStr
	if doc.OCRConfidence != nil {
		source.Confidence = doc.OCRConfidence
	}

	// Cargar perfil médico actual
	profile, err := w.medicalRepo.GetByUserID(ctx, userID)
	if err != nil {
		if err == domain.ErrMedicalProfileNotFound {
			// Crear perfil médico nuevo con los datos extraídos
			now := time.Now()
			profile = &domain.MedicalProfile{
				ID:        uuid.Must(uuid.NewV7()),
				UserID:    userID,
				Data:      make(map[string]*domain.MedicalFieldValue),
				IsShared:  false,
				CreatedAt: now,
				UpdatedAt: now,
			}
		} else {
			slog.Error("doc OCR: get medical profile failed", "user_id", userID, "error", err)
			return
		}
	}

	// Inicializar Data si es nil
	if profile.Data == nil {
		profile.Data = make(map[string]*domain.MedicalFieldValue)
	}

	now := time.Now()
	hasConflicts := false
	appliedFields := []string{}

	for ocrField, ocrValue := range extracted.MedicalFields {
		if ocrValue == "" {
			continue
		}

		profileField, exists := medicalFieldMap[ocrField]
		if !exists {
			continue
		}

		encKey := profileField + "_enc"
		existingField, hasExisting := profile.Data[encKey]

		if !hasExisting || existingField == nil || existingField.Value == "" {
			// Campo vacío → encriptar y auto-aplicar bajo _enc key
			encrypted, encErr := w.encryptionService.Encrypt(ocrValue)
			if encErr != nil {
				slog.Error("failed to encrypt OCR medical data", "field", profileField, "error", encErr)
				continue
			}
			encodedValue := base64.StdEncoding.EncodeToString(encrypted)
			profile.Data[encKey] = &domain.MedicalFieldValue{
				Value:     encodedValue,
				Source:    source,
				UpdatedAt: now,
			}
			appliedFields = append(appliedFields, profileField)
		} else {
			// Desencriptar valor existente para comparar
			ciphertext, decErr := base64.StdEncoding.DecodeString(existingField.Value)
			if decErr != nil {
				slog.Error("failed to decode existing medical field", "field", profileField, "error", decErr)
				hasConflicts = true
				continue
			}
			plainExisting, decErr := w.encryptionService.Decrypt(ciphertext)
			if decErr != nil {
				slog.Error("failed to decrypt existing medical field", "field", profileField, "error", decErr)
				hasConflicts = true
				continue
			}
			if plainExisting != ocrValue {
				// Conflicto → crear MedicalPendingUpdate
				hasConflicts = true
				pending := &domain.MedicalPendingUpdate{
					ID:               uuid.Must(uuid.NewV7()),
					UserID:           userID,
					SourceType:       "ocr",
					SourceDocumentID: &docID,
					FieldName:        profileField,
					CurrentValue:     &plainExisting,
					ProposedValue:    ocrValue,
					SuggestedAt:      now,
					ExpiresAt:        now.Add(30 * 24 * time.Hour),
					Status:           domain.PendingUpdatePending,
				}
				if err := w.pendingRepo.Create(ctx, pending); err != nil {
					slog.Error("doc OCR: create pending update failed", "field", profileField, "error", err)
				}
			}
		}
	}

	// Persistir perfil médico si hubo auto-aplicaciones
	if len(appliedFields) > 0 {
		if profile.CreatedAt.IsZero() {
			// Perfil nuevo → Create
			if err := w.medicalRepo.Create(ctx, profile); err != nil {
				slog.Error("doc OCR: create medical profile failed", "user_id", userID, "error", err)
			}
		} else {
			// Perfil existente → Update
			if err := w.medicalRepo.Update(ctx, profile); err != nil {
				slog.Error("doc OCR: update medical profile failed", "user_id", userID, "error", err)
			}
		}
	}

	// Actualizar metadata del documento con resumen médico
	summary := map[string]interface{}{
		"applied_fields":  appliedFields,
		"has_conflicts":   hasConflicts,
	}
	summaryJSON, _ := json.Marshal(summary)
	doc.MedicalUpdateSummary = summaryJSON
	doc.HasNewerMedicalData = hasConflicts

	if hasConflicts {
		slog.Info("doc OCR: medical conflicts detected", "doc_id", docID, "user_id", userID, "applied", appliedFields)
	}
	if len(appliedFields) > 0 {
		slog.Info("doc OCR: medical fields auto-applied", "doc_id", docID, "user_id", userID, "fields", appliedFields)
	}
}

// =============================================================================
// Helpers
// =============================================================================

// markFailed marca el documento como fallido con el motivo indicado.
func (w *OCRWorker) markFailed(ctx context.Context, doc *domain.UserDocument, reason string) {
	doc.OCRStatus = domain.OCRStatusFailed
	doc.FailureReason = &reason
	doc.UpdatedAt = time.Now()
	if err := w.docRepo.Update(ctx, doc); err != nil {
		slog.Error("doc OCR: mark failed update error", "doc_id", doc.ID, "error", err)
	}
	w.publishSSEEvent(ctx, doc.UserID, doc.ID, "failed", map[string]interface{}{
		"failure_reason": reason,
		"detail":         "El servicio OCR encontró un error técnico.",
	})
	w.updateDocStatusCache(ctx, doc.ID, "failed")
}

// markRejected marca el documento como rechazado.
func (w *OCRWorker) markRejected(ctx context.Context, doc *domain.UserDocument, reason string) {
	doc.OCRStatus = domain.OCRStatusRejected
	doc.FailureReason = &reason
	doc.UpdatedAt = time.Now()
	if err := w.docRepo.Update(ctx, doc); err != nil {
		slog.Error("doc OCR: mark rejected update error", "doc_id", doc.ID, "error", err)
	}
}

// updateDocStatusCache actualiza la cache de estado en Dragonfly.
func (w *OCRWorker) updateDocStatusCache(ctx context.Context, docID uuid.UUID, status string) {
	statusJSON, _ := json.Marshal(map[string]interface{}{
		"document_id": docID.String(),
		"status":      status,
	})
	w.rdb.SetEx(ctx, fmt.Sprintf("doc:status:%s", docID.String()), statusJSON, 1*time.Hour)
}

// publishSSEEvent publica un evento SSE en el stream doc:events:{id}
// y también en el SSE Hub para consumidores conectados vía HTTP.
func (w *OCRWorker) publishSSEEvent(ctx context.Context, userID, docID uuid.UUID, event string, data map[string]interface{}) {
	// Redis stream (existing — kept for other consumers)
	stream := fmt.Sprintf("{events}:doc:events:%s", docID.String())

	payload := map[string]interface{}{
		"event":     event,
		"timestamp": fmt.Sprintf("%d", time.Now().UnixMilli()),
	}
	for k, val := range data {
		payload[k] = val
	}

	if _, err := w.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     "*",
		Values: payload,
	}).Result(); err != nil {
		slog.Warn("doc OCR: publish SSE event failed", "doc_id", docID, "event", event, "error", err)
	}

	// SSE Hub (new — for HTTP EventSource connections)
	hubEvent := sse.Event{
		Type: "doc." + event,
		Data: payload,
	}
	sse.GetHub().Publish(userID, hubEvent)
}

// =============================================================================
// Orphan rescue
// =============================================================================

func (w *OCRWorker) rescueOrphans(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		messages, err := eventbus.RescueOrphanedMessages(ctx, w.rdb, docOCRStream, w.group, 5*time.Minute)
		if err != nil {
			slog.Error("doc OCR: rescue orphans error", "error", err)
			continue
		}

		for _, msg := range messages {
			slog.Info("doc OCR: reclaiming orphan message", "msg_id", msg.ID)
			w.processMessage(ctx, msg)
		}
	}
}


