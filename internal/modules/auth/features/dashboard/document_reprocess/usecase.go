// Lógica de negocio para reprocesamiento OCR de documentos desde el dashboard.
// DR-REQ-1: pone ocr_status = "queued", publica en {events}:doc:validate,
// actualiza cache doc:status, retorna 202 Accepted, idempotente.
package document_reprocess

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Puerto de repositorio — interfaz local que el adapter PG implementa
// =============================================================================

// DocumentReprocessRepo es el puerto local para reprocesamiento OCR.
// Implementado por el adapter postgres.DocumentRepository.
type DocumentReprocessRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.DocumentRow, error)
	UpdateOCRStatus(ctx context.Context, id uuid.UUID, status string) error
}

// =============================================================================
// Puerto de EventPublisher — interfaz local para publicar eventos
// =============================================================================

// EventPublisher es el puerto local para publicar eventos de dominio.
type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orquesta el reprocesamiento OCR de documentos con idempotencia,
// publicación al pipeline {events}:doc:validate y actualización de cache.
type UseCase struct {
	repo           DocumentReprocessRepo
	rdb            *redis.Client
	eventPublisher EventPublisher
}

// NewUseCase crea un nuevo use case de reprocesamiento de documentos.
func NewUseCase(repo DocumentReprocessRepo, rdb *redis.Client, eventPublisher EventPublisher) *UseCase {
	return &UseCase{repo: repo, rdb: rdb, eventPublisher: eventPublisher}
}

// =============================================================================
// Execute — POST: poner documento en cola de reprocesamiento OCR
// =============================================================================

// Execute pone un documento en cola para reprocesamiento OCR.
// Flow: validate → GetByID → idempotency check → UpdateOCRStatus("queued") →
//
//	cache status → publish to {events}:doc:validate → 202.
//
// DR-REQ-1: retorna 202 Accepted con status="queued".
// DR-REQ-1.3: si ya está "queued", retorna idempotentemente el mismo 202.
// Ciclo completo: cache (doc:status) → evento (Dragonfly Stream) → DB (ocr_status).
func (uc *UseCase) Execute(ctx context.Context, cmd ReprocessCommand) (*ReprocessResponse, error) {
	// 1. Validar
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Obtener documento
	doc, err := uc.repo.GetByID(ctx, cmd.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("get document for reprocess: %w", err)
	}

	// 3. Idempotencia: si ya está "queued", retornar mismo 202
	if doc.OCRStatus == "queued" {
		return &ReprocessResponse{
			DocumentID: cmd.DocumentID,
			Status:     "queued",
			Message:    "El documento ya está en cola de reprocesamiento",
		}, nil
	}

	// 4. Actualizar ocr_status a "queued" en PostgreSQL
	if err := uc.repo.UpdateOCRStatus(ctx, cmd.DocumentID, "queued"); err != nil {
		return nil, fmt.Errorf("update ocr status for reprocess: %w", err)
	}

	// 5. Actualizar cache doc:status:{id} en Dragonfly (best-effort)
	uc.updateStatusCache(doc)

	// 6. Publicar en Dragonfly Stream {events}:doc:validate para reingresar al pipeline
	//    El pipeline: Validator → Sanitizer → OCR Worker
	//    Fire-and-forget: no bloquea la respuesta 202
	uc.publishToPipeline(doc)

	return &ReprocessResponse{
		DocumentID: cmd.DocumentID,
		Status:     "queued",
		Message:    "Documento encolado para reprocesamiento OCR",
	}, nil
}

// =============================================================================
// Cache status — doc:status:{id} (best-effort, TTL 1h)
// =============================================================================

// updateStatusCache actualiza la entrada doc:status:{id} en Dragonfly.
// Es best-effort: si falla, el TTL del cache (1h) eventualmente expira.
// El cache existe para que el frontend pueda consultar el estado sin esperar al SSE.
func (uc *UseCase) updateStatusCache(doc *domain.DocumentRow) {
	if uc.rdb == nil {
		return
	}

	statusJSON, err := json.Marshal(map[string]interface{}{
		"document_id":        doc.ID.String(),
		"status":             "queued",
		"file_name":          doc.FileName,
		"mime_type":          doc.MimeType,
		"verification_status": doc.VerificationStatus,
		"reprocess":          true,
	})
	if err != nil {
		return
	}

	cacheKey := fmt.Sprintf("doc:status:%s", doc.ID.String())
	if err := uc.rdb.SetEx(context.Background(), cacheKey, statusJSON, 1*time.Hour).Err(); err != nil {
		// best-effort — el pipeline actualizará este cache cuando procese
		_ = err
	}
}

// =============================================================================
// Publicación al pipeline — fire-and-forget (goroutine + 2s timeout)
// =============================================================================

// publishToPipeline publica el evento en {events}:doc:validate para que
// el pipeline de OCR (Validator → Sanitizer → OCR Worker) reprocese el documento.
// El payload replica los campos que espera el ValidatorWorker:
//
//	required: document_id, storage_key
//	optional: user_id, file_name, detected_mime_type
//
// Fire-and-forget: no bloquea la respuesta HTTP 202.
func (uc *UseCase) publishToPipeline(doc *domain.DocumentRow) {
	if uc.eventPublisher == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		stream := eventbus.StreamName("doc:validate") // → {events}:doc:validate
		now := time.Now().UnixMilli()
		payload := map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            doc.UserID.String(),
			"storage_key":        doc.StorageKey,
			"file_name":          doc.FileName,
			"detected_mime_type": doc.MimeType,
			"reprocess":          "true",
			"timestamp":          fmt.Sprintf("%d", now),
		}

		if _, err := uc.eventPublisher.Publish(ctx, stream, payload); err != nil {
			// fire-and-forget — el admin puede reintentar via POST /reprocess
			_ = err
		}
	}()
}
