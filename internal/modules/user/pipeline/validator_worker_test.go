// Tests del ValidatorWorker: validación de documentos vía Dragonfly Streams.
// Simula miniredis, mockea DocumentUpdater y ValidatorR2Client.
package pipeline_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/pipeline"
	sse "github.com/ProacTrip/Backend/internal/shared/sse"
)

// =============================================================================
// Mocks: DocumentUpdater + ValidatorR2Client
// =============================================================================

type validatorMockDocRepo struct {
	mu      sync.Mutex
	docs    map[uuid.UUID]*domain.UserDocument
	getErr  error
	updErr  error
	updated []*domain.UserDocument
}

func newValidatorMockDocRepo() *validatorMockDocRepo {
	return &validatorMockDocRepo{docs: make(map[uuid.UUID]*domain.UserDocument)}
}

func (m *validatorMockDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	doc, ok := m.docs[id]
	if !ok {
		return nil, domain.ErrDocumentNotFound
	}
	return doc, nil
}

func (m *validatorMockDocRepo) Update(ctx context.Context, doc *domain.UserDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updErr != nil {
		return m.updErr
	}
	// Guardar copia en el map
	cpy := *doc
	m.docs[doc.ID] = &cpy
	m.updated = append(m.updated, doc)
	return nil
}

// validatorMockR2 implementa ValidatorR2Client.
type validatorMockR2 struct {
	downloadData  []byte
	downloadErr   error
	contentType   string
	headErr       error
	downloadCalls int
	headCalls     int
}

func (m *validatorMockR2) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	m.downloadCalls++
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	return io.NopCloser(bytes.NewReader(m.downloadData)), nil
}

func (m *validatorMockR2) HeadContentType(ctx context.Context, bucket, key string) (string, error) {
	m.headCalls++
	if m.headErr != nil {
		return "", m.headErr
	}
	return m.contentType, nil
}

// =============================================================================
// Helper: crear documento en estado queued
// =============================================================================

func newQueuedDoc(userID uuid.UUID, docTypeID uuid.UUID, mime string) *domain.UserDocument {
	doc := domain.NewUserDocument(userID, &docTypeID, "test.pdf", "raw/test.pdf", mime)
	doc.OCRStatus = domain.OCRStatusQueued
	return doc
}

// =============================================================================
// Test: flujo exitoso de validación con documento JPEG
// =============================================================================

func TestValidatorWorker_ProcesaJPEGCorrectamente(t *testing.T) {
	sse.Init(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := newQueuedDoc(userID, docTypeID, "image/jpeg")

	docRepo := newValidatorMockDocRepo()
	docRepo.docs[doc.ID] = doc

	// Mock R2: archivo JPEG válido, content type coincide
	r2Mock := &validatorMockR2{
		// Magic bytes JPEG: FF D8 FF
		downloadData: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00},
		contentType:  "image/jpeg",
	}

	worker := pipeline.NewValidatorWorker(rdb, docRepo, r2Mock)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}
	defer cancel()

	// Publicar mensaje de validación
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "raw/test.jpg",
			"file_name":          "test.jpg",
			"detected_mime_type": "image/jpeg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	// Esperar procesamiento vía output stream (reemplaza time.Sleep)
	<-time.After(200 * time.Millisecond)  // esperar procesamiento (sin output en error cases)

	// Verificar que el documento pasó a sanitizing
	docRepo.mu.Lock()
	updated, ok := docRepo.docs[doc.ID]
	docRepo.mu.Unlock()

	if !ok {
		t.Fatal("documento no encontrado en el repo")
	}
	if updated.OCRStatus != domain.OCRStatusSanitizing {
		t.Errorf("OCRStatus = %q, want %q", updated.OCRStatus, domain.OCRStatusSanitizing)
	}
	if updated.DetectedMimeType == nil || *updated.DetectedMimeType != "image/jpeg" {
		t.Errorf("DetectedMimeType no fue seteado correctamente")
	}
}

// =============================================================================
// Test: MIME no aceptado → documento rechazado
// =============================================================================

func TestValidatorWorker_MIMENoAceptado_RechazaDocumento(t *testing.T) {
	sse.Init(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := newQueuedDoc(userID, docTypeID, "image/gif")

	docRepo := newValidatorMockDocRepo()
	docRepo.docs[doc.ID] = doc

	r2Mock := &validatorMockR2{} // no debería usarse porque el MIME es rechazado antes

	worker := pipeline.NewValidatorWorker(rdb, docRepo, r2Mock)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "raw/test.gif",
			"file_name":          "test.gif",
			"detected_mime_type": "image/gif", // NO aceptado
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(200 * time.Millisecond)  // esperar procesamiento (sin output en error cases)

	// Verificar que el documento fue rechazado
	docRepo.mu.Lock()
	updated, ok := docRepo.docs[doc.ID]
	docRepo.mu.Unlock()

	if !ok {
		t.Fatal("documento no encontrado en el repo")
	}
	if updated.OCRStatus != domain.OCRStatusRejected {
		t.Errorf("OCRStatus = %q, want %q", updated.OCRStatus, domain.OCRStatusRejected)
	}
	if updated.FailureReason == nil {
		t.Error("debería tener FailureReason seteado")
	}
}

// =============================================================================
// Test: missing document_id → XACK inmediato
// =============================================================================

func TestValidatorWorker_FaltaDocumentID_RechazaInmediato(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docRepo := newValidatorMockDocRepo()
	r2Mock := &validatorMockR2{}

	worker := pipeline.NewValidatorWorker(rdb, docRepo, r2Mock)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"user_id":     uuid.Must(uuid.NewV7()).String(),
			"storage_key": "raw/test.jpg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(200 * time.Millisecond)  // esperar procesamiento (sin output en error cases)

	// Verificar que el mensaje fue ACKeado (no debería estar en PEL)
	pending, err := rdb.XPending(ctx, "{events}:doc:validate", "doc-validate-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count > 0 {
		t.Errorf("mensaje sin document_id debería ser ACKeado, PEL count=%d", pending.Count)
	}
}

// =============================================================================
// Test: documento no encontrado en DB → XACK
// =============================================================================

func TestValidatorWorker_DocumentoNoEncontrado_ACK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docRepo := newValidatorMockDocRepo() // repo vacío
	r2Mock := &validatorMockR2{}

	worker := pipeline.NewValidatorWorker(rdb, docRepo, r2Mock)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	docID := uuid.Must(uuid.NewV7())
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        docID.String(),
			"user_id":            uuid.Must(uuid.NewV7()).String(),
			"storage_key":        "raw/test.jpg",
			"detected_mime_type": "image/jpeg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(200 * time.Millisecond)  // esperar procesamiento (sin output en error cases)

	pending, err := rdb.XPending(ctx, "{events}:doc:validate", "doc-validate-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count > 0 {
		t.Errorf("documento inexistente debería ser ACKeado, PEL count=%d", pending.Count)
	}
}

// =============================================================================
// Test: cross-validation falla por extensión incorrecta
// =============================================================================

func TestValidatorWorker_ExtensionIncorrecta_RechazaDocumento(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := newQueuedDoc(userID, docTypeID, "image/jpeg")

	docRepo := newValidatorMockDocRepo()
	docRepo.docs[doc.ID] = doc

	r2Mock := &validatorMockR2{
		downloadData: []byte{0xFF, 0xD8, 0xFF},
		contentType:  "image/jpeg",
	}

	worker := pipeline.NewValidatorWorker(rdb, docRepo, r2Mock)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	// storage_key termina en .png pero MIME es image/jpeg → debe fallar cross-validation
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "raw/test.png", // extensión no coincide con JPEG
			"file_name":          "test.png",
			"detected_mime_type": "image/jpeg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(200 * time.Millisecond)  // esperar procesamiento (sin output en error cases)

	docRepo.mu.Lock()
	updated, ok := docRepo.docs[doc.ID]
	docRepo.mu.Unlock()

	if !ok {
		t.Fatal("documento no encontrado en el repo")
	}
	if updated.OCRStatus != domain.OCRStatusRejected {
		t.Errorf("OCRStatus = %q, want %q (extensión incorrecta debe rechazar)",
			updated.OCRStatus, domain.OCRStatusRejected)
	}
}

// =============================================================================
// Test: error de conexión a DB → mensaje queda en PEL (no XACK)
// =============================================================================

func TestValidatorWorker_ErrorDB_NoHaceXACK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docRepo := newValidatorMockDocRepo()
	docRepo.getErr = errors.New("database connection lost")

	r2Mock := &validatorMockR2{}

	worker := pipeline.NewValidatorWorker(rdb, docRepo, r2Mock)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	docID := uuid.Must(uuid.NewV7())
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:validate",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        docID.String(),
			"user_id":            uuid.Must(uuid.NewV7()).String(),
			"storage_key":        "raw/test.jpg",
			"detected_mime_type": "image/jpeg",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	// DB error → worker no produce output event. Espera corta para procesamiento.
	<-time.After(200 * time.Millisecond)

	// El mensaje DEBERÍA estar en PEL porque el worker no hizo XACK
	pending, err := rdb.XPending(ctx, "{events}:doc:validate", "doc-validate-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count == 0 {
		t.Error("mensaje debería estar en PEL (DB error → no XACK)")
	}
}
