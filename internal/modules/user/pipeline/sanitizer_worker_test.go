// Tests del SanitizerWorker: sanitización de documentos vía Dragonfly Streams.
// Simula miniredis, mockea DocumentUpdater y SanitizerR2Client.
package pipeline_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/pipeline"
)

// =============================================================================
// Mocks: DocumentUpdater + SanitizerR2Client
// =============================================================================

type sanitizerMockDocRepo struct {
	mu      sync.Mutex
	docs    map[uuid.UUID]*domain.UserDocument
	getErr  error
	updErr  error
	updated []*domain.UserDocument
}

func newSanitizerMockDocRepo() *sanitizerMockDocRepo {
	return &sanitizerMockDocRepo{docs: make(map[uuid.UUID]*domain.UserDocument)}
}

func (m *sanitizerMockDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
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

func (m *sanitizerMockDocRepo) Update(ctx context.Context, doc *domain.UserDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updErr != nil {
		return m.updErr
	}
	cpy := *doc
	m.docs[doc.ID] = &cpy
	m.updated = append(m.updated, doc)
	return nil
}

// sanitizerMockR2 implementa SanitizerR2Client.
type sanitizerMockR2 struct {
	mu           sync.Mutex
	files        map[string][]byte // key → contenido
	downloadErr  error
	uploadErr    error
	deleteErr    error
	uploadCalls  int
	deleteCalls  int
}

func newSanitizerMockR2() *sanitizerMockR2 {
	return &sanitizerMockR2{files: make(map[string][]byte)}
}

func (m *sanitizerMockR2) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	data, ok := m.files[key]
	if !ok {
		return nil, errors.New("file not found in R2")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *sanitizerMockR2) Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploadCalls++
	if m.uploadErr != nil {
		return m.uploadErr
	}
	data, _ := io.ReadAll(reader)
	m.files[key] = data
	return nil
}

func (m *sanitizerMockR2) Delete(ctx context.Context, bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls++
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

// =============================================================================
// Test: flujo exitoso de sanitización para PNG
// =============================================================================

func TestSanitizerWorker_ProcesaPNGCorrectamente(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := domain.NewUserDocument(userID, docTypeID, "test.png", "raw/test.png", "image/png")
	doc.OCRStatus = domain.OCRStatusSanitizing

	docRepo := newSanitizerMockDocRepo()
	docRepo.docs[doc.ID] = doc

	// Crear una imagen PNG real de 2×2 píxeles para sanitización
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	png.Encode(&buf, img)
	pngData := buf.Bytes()

	r2Mock := newSanitizerMockR2()
	r2Mock.files["raw/test.png"] = pngData

	worker := pipeline.NewSanitizerWorker(rdb, r2Mock, docRepo)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}
	defer cancel()

	// Publicar mensaje de sanitización
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:sanitize",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "raw/test.png",
			"file_name":          "test.png",
			"detected_mime_type": "image/png",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verificar estado del documento
	docRepo.mu.Lock()
	updated, ok := docRepo.docs[doc.ID]
	docRepo.mu.Unlock()

	if !ok {
		t.Fatal("documento no encontrado en el repo")
	}
	if updated.OCRStatus != domain.OCRStatusOCRProcessing {
		t.Errorf("OCRStatus = %q, want %q", updated.OCRStatus, domain.OCRStatusOCRProcessing)
	}

	// Verificar que se subió el archivo sanitizado a R2
	r2Mock.mu.Lock()
	uploadCalls := r2Mock.uploadCalls
	r2Mock.mu.Unlock()

	if uploadCalls == 0 {
		t.Error("upload a R2 nunca fue llamado")
	}

	// Verificar que el mensaje fue producido al stream doc:ocr
	ocrLen, err := rdb.XLen(ctx, "{events}:doc:ocr").Result()
	if err != nil {
		t.Fatalf("XLen OCR falló: %v", err)
	}
	if ocrLen == 0 {
		t.Error("mensaje NO fue producido al stream doc:ocr")
	}

	// Verificar que el storage key fue actualizado a la ruta processed/
	if !contains(updated.StorageKey, "processed/") {
		t.Errorf("StorageKey = %q, debería contener 'processed/'", updated.StorageKey)
	}
}

// =============================================================================
// Test: falta document_id → XACK inmediato
// =============================================================================

func TestSanitizerWorker_FaltaDocumentID_RechazaInmediato(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docRepo := newSanitizerMockDocRepo()
	r2Mock := newSanitizerMockR2()

	worker := pipeline.NewSanitizerWorker(rdb, r2Mock, docRepo)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:sanitize",
		ID:     "*",
		Values: map[string]interface{}{
			"user_id":     uuid.Must(uuid.NewV7()).String(),
			"storage_key": "raw/test.png",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	pending, err := rdb.XPending(ctx, "{events}:doc:sanitize", "doc-sanitize-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count > 0 {
		t.Errorf("mensaje sin document_id debería ser ACKeado, PEL count=%d", pending.Count)
	}
}

// =============================================================================
// Test: documento no encontrado → XACK
// =============================================================================

func TestSanitizerWorker_DocumentoNoEncontrado_ACK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docRepo := newSanitizerMockDocRepo() // vacío
	r2Mock := newSanitizerMockR2()

	worker := pipeline.NewSanitizerWorker(rdb, r2Mock, docRepo)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	docID := uuid.Must(uuid.NewV7())
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:sanitize",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id": docID.String(),
			"user_id":     uuid.Must(uuid.NewV7()).String(),
			"storage_key": "raw/test.png",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	pending, err := rdb.XPending(ctx, "{events}:doc:sanitize", "doc-sanitize-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count > 0 {
		t.Errorf("documento inexistente debería ser ACKeado, PEL count=%d", pending.Count)
	}
}

// =============================================================================
// Test: falla download de R2 → mensaje queda en PEL (no XACK)
// =============================================================================

func TestSanitizerWorker_FalloDownload_QuedaEnPEL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := domain.NewUserDocument(userID, docTypeID, "test.png", "raw/test.png", "image/png")
	doc.OCRStatus = domain.OCRStatusSanitizing

	docRepo := newSanitizerMockDocRepo()
	docRepo.docs[doc.ID] = doc

	r2Mock := newSanitizerMockR2()
	r2Mock.downloadErr = errors.New("R2 connection timeout")

	worker := pipeline.NewSanitizerWorker(rdb, r2Mock, docRepo)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:sanitize",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "raw/test.png",
			"file_name":          "test.png",
			"detected_mime_type": "image/png",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	pending, err := rdb.XPending(ctx, "{events}:doc:sanitize", "doc-sanitize-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count == 0 {
		t.Error("mensaje debería estar en PEL porque download falló (reintento pendiente)")
	}
}

// =============================================================================
// Test: sanitización de PDF elimina entradas peligrosas
// =============================================================================

func TestSanitizerWorker_SanitizaPDF_EliminaPeligroso(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := domain.NewUserDocument(userID, docTypeID, "test.pdf", "raw/test.pdf", "application/pdf")
	doc.OCRStatus = domain.OCRStatusSanitizing

	docRepo := newSanitizerMockDocRepo()
	docRepo.docs[doc.ID] = doc

	// PDF con contenido peligroso (JavaScript)
	pdfContent := []byte("%PDF-1.7\n/JS (alert('hack'))\n/OpenAction << /S /JavaScript /JS (alert) >>\n%%EOF")
	r2Mock := newSanitizerMockR2()
	r2Mock.files["raw/test.pdf"] = pdfContent

	worker := pipeline.NewSanitizerWorker(rdb, r2Mock, docRepo)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:sanitize",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "raw/test.pdf",
			"file_name":          "test.pdf",
			"detected_mime_type": "application/pdf",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verificar que el archivo sanitizado NO contiene entradas peligrosas
	r2Mock.mu.Lock()
	defer r2Mock.mu.Unlock()

	for key := range r2Mock.files {
		if contains(key, "processed/") {
			data := r2Mock.files[key]
			if bytes.Contains(data, []byte("/JS")) {
				t.Errorf("archivo sanitizado aún contiene /JS: %q", string(data))
			}
			if bytes.Contains(data, []byte("/OpenAction")) {
				t.Errorf("archivo sanitizado aún contiene /OpenAction")
			}
		}
	}
}

// =============================================================================
// Test: malformed event (UUID inválido) → XACK
// =============================================================================

func TestSanitizerWorker_UUIDInvalido_ACK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docRepo := newSanitizerMockDocRepo()
	r2Mock := newSanitizerMockR2()

	worker := pipeline.NewSanitizerWorker(rdb, r2Mock, docRepo)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:sanitize",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id": "no-es-uuid-valido",
			"user_id":     uuid.Must(uuid.NewV7()).String(),
			"storage_key": "raw/test.png",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	pending, err := rdb.XPending(ctx, "{events}:doc:sanitize", "doc-sanitize-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count > 0 {
		t.Errorf("mensaje con UUID inválido debería ser ACKeado, PEL count=%d", pending.Count)
	}
}

// =============================================================================
// Helper
// =============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && bytes.Contains([]byte(s), []byte(substr))
}
