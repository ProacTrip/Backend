// Caso de uso: Subir documento al pipeline asíncrono.
// Order: Auth → Rate Limit → Content-Length → Max docs → Magic bytes → blake3 → Dedup → Upload
package upload_document

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/filetype"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Constantes
// =============================================================================

const (
	dedupGlobalTTL  = 7 * 24 * time.Hour
	dedupLockTTL    = 30 * time.Second
	dedupLockWait   = 2 * time.Second
)

// Lua script for atomic rate limit with hashtag.
var rateLimitScript = redis.NewScript(`
	local current = redis.call('INCR', KEYS[1])
	if current == 1 then
		redis.call('EXPIRE', KEYS[1], ARGV[1])
	end
	return current
`)

// Lua script for safe lock release — only deletes if the value matches.
// Prevents race condition where a deferred Del removes a lock held by another worker.
var dedupUnlockScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	else
		return 0
	end
`)

// =============================================================================
// Ports
// =============================================================================

// DocumentRepo define el puerto para operaciones de documento en PostgreSQL.
type DocumentRepo interface {
	Create(ctx context.Context, doc *domain.UserDocument) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	CountByUserID(ctx context.Context, userID uuid.UUID) (int, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// DocR2Storage define el puerto para subir archivos a R2.
type DocR2Storage interface {
	Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, bucket, key string) error
}

// DocEventPublisher define el puerto para publicar eventos en Dragonfly Streams.
type DocEventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	DocRepo             DocumentRepo
	Storage             DocR2Storage
	EventPublisher      DocEventPublisher
	Dragonfly           *redis.Client
	MaxDocumentsPerUser int
	RateLimitMax        int
	RateLimitWindowSecs int
}

// UseCase implementa la subida de documentos.
type UseCase struct {
	docRepo             DocumentRepo
	storage             DocR2Storage
	eventPublisher      DocEventPublisher
	dragonfly           *redis.Client
	maxDocumentsPerUser int
	rateLimitMax        int
	rateLimitWindowSecs int
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	maxDocs := deps.MaxDocumentsPerUser
	if maxDocs <= 0 {
		maxDocs = 5
	}
	rlMax := deps.RateLimitMax
	if rlMax <= 0 {
		rlMax = 10
	}
	rlWindow := deps.RateLimitWindowSecs
	if rlWindow <= 0 {
		rlWindow = 60
	}
	return &UseCase{
		docRepo:             deps.DocRepo,
		storage:             deps.Storage,
		eventPublisher:      deps.EventPublisher,
		dragonfly:           deps.Dragonfly,
		maxDocumentsPerUser: maxDocs,
		rateLimitMax:        rlMax,
		rateLimitWindowSecs: rlWindow,
	}
}

// deducedupGlobalData contains cached global dedup info.
type dedupGlobalData struct {
	DocumentID string          `json:"document_id"`
	StorageKey string          `json:"storage_key"`
	OCRResults json.RawMessage `json:"ocr_results"`
}

// CheckRateLimit verifica el rate limit para el usuario dado.
// Debe llamarse en el handler ANTES de Content-Length check.
// Rate limit is the cheapest check — protects against spam before CPU/IO.
func (uc *UseCase) CheckRateLimit(ctx context.Context, userIDStr string) error {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user_id for rate limit: %w", err)
	}
	rateKey := fmt.Sprintf("{ratelimit}:doc:upload:%s", userID.String())
	allowed, err := uc.checkRateLimit(ctx, rateKey, uc.rateLimitMax, uc.rateLimitWindowSecs)
	if err != nil {
		slog.Error("rate limit check failed", "user_id", userID, "error", err)
		return nil // degradar gracefully — permitir si falla la verificación
	}
	if !allowed {
		return domain.ErrRateLimitExceeded
	}
	return nil
}

// Execute procesa la subida del documento.
// Order: Magic bytes → MIME detection → Size validation → Max docs → blake3 → Dedup → Upload
func (uc *UseCase) Execute(ctx context.Context, cmd UploadDocumentCommand) (*UploadDocumentResponse, error) {
	// 1. Parsear userID (auth verified by handler)
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	realSize := int64(len(cmd.FileBytes))
	if realSize == 0 {
		return nil, domain.ErrFileTooLarge
	}

	// 2. Magic bytes detection — el usecase detecta el MIME type real
	headerSize := 512
	if len(cmd.FileBytes) < headerSize {
		headerSize = len(cmd.FileBytes)
	}
	detectedMime, err := filetype.DetectMimeType(cmd.FileBytes[:headerSize])
	if err != nil || !filetype.IsAccepted(detectedMime) {
		return nil, domain.ErrInvalidFileType
	}
	cmd.MimeType = detectedMime // establecido para que reuseGlobalDedup y demás lo consuman

	// 3. Size validation según MIME type detectado
	maxSize := MaxSizeForMIME(detectedMime)
	if realSize > maxSize {
		return nil, fmt.Errorf("FILE_TOO_LARGE: %d bytes exceeds max %d bytes for %s: %w",
			realSize, maxSize, detectedMime, domain.ErrFileTooLarge)
	}

	// 4. Max documents per user check (count ALL docs, all statuses)
	count, err := uc.docRepo.CountByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("contar documentos del usuario: %w", err)
	}
	if count >= uc.maxDocumentsPerUser {
		return nil, domain.ErrMaxDocumentsReached
	}

	// 5. blake3 content hash (for dedup)
	contentHash := filetype.ContentHash(cmd.FileBytes)

	// Si Dragonfly no está configurado, los dedup checks no pueden realizarse
	if uc.dragonfly == nil {
		return nil, fmt.Errorf("dragonfly client is required for dedup")
	}

	// 6. Dedup — per-user reject
	dedupUserKey := fmt.Sprintf("{dedup}:user:%s:%s", userID.String(), contentHash)
	exists, err := uc.dragonfly.Exists(ctx, dedupUserKey).Result()
	if err != nil {
		slog.Error("dedup user check failed", "user_id", userID, "error", err)
	} else if exists > 0 {
		return nil, domain.ErrDuplicateDocument
	}

	// 7. Dedup — global reuse with SETNX lock to prevent race condition
	dedupGlobalKey := fmt.Sprintf("{dedup}:global:%s", contentHash)
	lockKey := fmt.Sprintf("{dedup}:lock:%s", contentHash)

	// Generate docID early so we can use it as lock value
	docID := uuid.Must(uuid.NewV7())
	storageKey := filetype.StorageKey(userID, docID, detectedMime)

	// Try to acquire dedup lock
	acquired, err := uc.dragonfly.SetNX(ctx, lockKey, docID.String(), dedupLockTTL).Result()
	if err != nil {
		slog.Error("dedup lock acquire failed", "hash", contentHash, "error", err)
		// Fall through to normal upload on lock error
	} else if !acquired {
		// Another upload is processing this hash — wait and recheck
		slog.Info("dedup lock not acquired, waiting for concurrent upload", "hash", contentHash)
		lockCtx, cancel := context.WithTimeout(ctx, dedupLockWait)
		defer cancel()
		timer := time.NewTimer(dedupLockWait)
		defer timer.Stop()
		select {
		case <-lockCtx.Done():
		case <-timer.C:
		}

		globalData, getErr := uc.dragonfly.Get(ctx, dedupGlobalKey).Result()
		if getErr == nil && globalData != "" {
			if resp, reused := uc.reuseGlobalDedup(ctx, userID, cmd, contentHash, dedupGlobalKey, globalData); reused {
				return resp, nil
			}
		}
		// If lock wait timed out or global dedup data not found, fall through to normal upload
		// Re-acquire lock for this upload attempt
		acquired2, err2 := uc.dragonfly.SetNX(ctx, lockKey, docID.String(), dedupLockTTL).Result()
		if err2 != nil || !acquired2 {
			return nil, domain.ErrDuplicateDocument
		}
		// Lock acquired on retry — proceed with upload
	} else {
		// Lock acquired — check global dedup under lock protection
		globalData, getErr := uc.dragonfly.Get(ctx, dedupGlobalKey).Result()
		if getErr == nil && globalData != "" {
			// Release lock since another upload already did the work
			uc.dragonfly.Del(ctx, lockKey)
			if resp, reused := uc.reuseGlobalDedup(ctx, userID, cmd, contentHash, dedupGlobalKey, globalData); reused {
				return resp, nil
			}
			// Corrupt cache data — re-acquire lock and proceed
			acquired2, err2 := uc.dragonfly.SetNX(ctx, lockKey, docID.String(), dedupLockTTL).Result()
			if err2 != nil || !acquired2 {
				return nil, domain.ErrDuplicateDocument
			}
		}
	}

	// Ensure lock is released on exit — atomic check prevents deleting another worker's lock
	defer func() {
		result, err := dedupUnlockScript.Run(ctx, uc.dragonfly, []string{lockKey}, docID.String()).Int64()
		if err != nil {
			slog.Warn("failed to release dedup lock", "key", lockKey, "doc_id", docID, "error", err)
		} else if result == 0 {
			slog.Warn("dedup lock not released — held by another worker or expired", "key", lockKey, "doc_id", docID)
		}
	}()

	// 8. Crear registro en DB con status "queued"
	now := time.Now()
	doc := &domain.UserDocument{
		ID:                docID,
		UserID:            userID,
		DocumentTypeID:    uuid.Nil, // será refinado por el pipeline
		FileName:          cmd.FileName,
		StorageKey:        storageKey,
		MimeType:          &detectedMime,
		DetectedMimeType:  &detectedMime,
		DetectedSizeBytes: &realSize,
		OCRStatus:         domain.OCRStatusQueued,
		IsVerified:        false,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	fs := int(realSize)
	doc.FileSize = &fs

	if err := uc.docRepo.Create(ctx, doc); err != nil {
		return nil, fmt.Errorf("crear documento en DB: %w", err)
	}

	// 9. Subir archivo a R2 raw/
	if err := uc.storage.Upload(ctx, "proactrip-secure", storageKey,
		bytes.NewReader(cmd.FileBytes), realSize, detectedMime); err != nil {
		slog.Error("fallo al subir archivo a R2", "doc_id", docID, "error", err)
		// Best-effort cleanup del registro huérfano en DB — si R2 falla
		// después de crear el registro, lo borramos para no dejar basura.
		if delErr := uc.docRepo.Delete(ctx, docID); delErr != nil {
			slog.Warn("no se pudo limpiar registro huérfano", "doc_id", docID, "error", delErr)
		}
		return nil, fmt.Errorf("subir archivo a R2: %w", err)
	}

	// 10. Set dedup keys after successful upload
	// user dedup: permanente (sin TTL)
	uc.dragonfly.Set(ctx, dedupUserKey, docID.String(), 0)
	// global dedup: 7d TTL
	globalPayload, _ := json.Marshal(dedupGlobalData{
		DocumentID: docID.String(),
		StorageKey: storageKey,
	})
	uc.dragonfly.SetEx(ctx, dedupGlobalKey, globalPayload, dedupGlobalTTL)

	// 11. Cache status en Dragonfly: doc:status:{id}
	statusJSON, _ := json.Marshal(map[string]interface{}{
		"document_id": docID.String(),
		"status":      string(domain.OCRStatusQueued),
		"file_name":   cmd.FileName,
		"mime_type":   detectedMime,
	})
	uc.dragonfly.SetEx(ctx, fmt.Sprintf("doc:status:%s", docID.String()), statusJSON, 1*time.Hour)

	// 12. Publicar en Dragonfly Stream: {events}:doc:validate
	stream := "{events}:doc:validate"
	payload := map[string]interface{}{
		"document_id":        docID.String(),
		"user_id":            userID.String(),
		"storage_key":        storageKey,
		"file_name":          cmd.FileName,
		"detected_mime_type": detectedMime,
		"content_hash":       contentHash,
		"timestamp":          fmt.Sprintf("%d", now.UnixMilli()),
	}

	if _, err := uc.eventPublisher.Publish(ctx, stream, payload); err != nil {
		slog.Error("fallo al publicar evento doc:validate", "doc_id", docID, "error", err)
	}

	// 13. Construir respuesta 202
	resp := &UploadDocumentResponse{
		DocumentID: docID.String(),
		Status:     string(domain.OCRStatusQueued),
		EventsURL:  fmt.Sprintf("/v1/user/documents/%s/events", docID.String()),
		Message:    "Documento recibido. El procesamiento ha comenzado. Seguí el progreso vía events_url.",
	}

	return resp, nil
}

// checkRateLimit runs the atomic Lua rate limit script.
func (uc *UseCase) checkRateLimit(ctx context.Context, key string, maxReqs, windowSecs int) (bool, error) {
	current, err := rateLimitScript.Run(ctx, uc.dragonfly, []string{key}, windowSecs).Int()
	if err != nil {
		return false, fmt.Errorf("rate limit script: %w", err)
	}
	return current <= maxReqs, nil
}

// reuseGlobalDedup creates a new DB record reusing storage_key from a global dedup hit.
// Returns (response, true) on success, (nil, false) if corrupt data — caller should fall through.
func (uc *UseCase) reuseGlobalDedup(
	ctx context.Context,
	userID uuid.UUID,
	cmd UploadDocumentCommand,
	contentHash string,
	dedupGlobalKey string,
	globalDataStr string,
) (*UploadDocumentResponse, bool) {
	var cached dedupGlobalData
	if err := json.Unmarshal([]byte(globalDataStr), &cached); err != nil {
		slog.Warn("corrupt global dedup data, falling through to normal upload", "error", err)
		return nil, false
	}

	docID := uuid.Must(uuid.NewV7())
	realSize := int64(len(cmd.FileBytes))

	now := time.Now()
	doc := &domain.UserDocument{
		ID:                docID,
		UserID:            userID,
		DocumentTypeID:    uuid.Nil,
		FileName:          cmd.FileName,
		StorageKey:        cached.StorageKey, // reuse same storage key
		MimeType:          &cmd.MimeType,
		DetectedMimeType:  &cmd.MimeType,
		DetectedSizeBytes: &realSize,
		OCRStatus:         domain.OCRStatusQueued,
		OCRData:           cached.OCRResults,
		IsVerified:        false,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	fs := int(realSize)
	doc.FileSize = &fs

	if err := uc.docRepo.Create(ctx, doc); err != nil {
		slog.Error("fallo al crear documento dedup en DB", "doc_id", docID, "error", err)
		return nil, false
	}

	// Set user dedup key (permanent)
	dedupUserKey := fmt.Sprintf("{dedup}:user:%s:%s", userID.String(), contentHash)
	uc.dragonfly.Set(ctx, dedupUserKey, docID.String(), 0)

	// Cache status
	statusJSON, _ := json.Marshal(map[string]interface{}{
		"document_id": docID.String(),
		"status":      string(domain.OCRStatusQueued),
		"file_name":   cmd.FileName,
		"mime_type":   cmd.MimeType,
		"reused":      true,
	})
	uc.dragonfly.SetEx(ctx, fmt.Sprintf("doc:status:%s", docID.String()), statusJSON, 1*time.Hour)

	// Publish validate event (still needs validation + OCR)
	stream := "{events}:doc:validate"
	payload := map[string]interface{}{
		"document_id":        docID.String(),
		"user_id":            userID.String(),
		"storage_key":        cached.StorageKey,
		"file_name":          cmd.FileName,
		"detected_mime_type": cmd.MimeType,
		"content_hash":       contentHash,
		"reused":             "true",
		"timestamp":          fmt.Sprintf("%d", now.UnixMilli()),
	}

	if _, err := uc.eventPublisher.Publish(ctx, stream, payload); err != nil {
		slog.Error("fallo al publicar evento doc:validate (reused)", "doc_id", docID, "error", err)
	}

	return &UploadDocumentResponse{
		DocumentID: docID.String(),
		Status:     string(domain.OCRStatusQueued),
		EventsURL:  fmt.Sprintf("/v1/user/documents/%s/events", docID.String()),
		Message:    "Documento reutilizado por deduplicación global. Procesamiento iniciado.",
	}, true
}
