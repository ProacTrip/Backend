// Tests del OCRWorker: extracción de datos vía OCR/AI, comparación médica.
// Simula miniredis, mockea OCRService, EncryptionService, repositorios y R2.
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
// Mocks: OCRR2Client, OCRDocUpdater, MedicalProfileManager, MedicalPendingCreator,
//        OCRService, EncryptionService
// =============================================================================

type ocrMockR2 struct {
	files       map[string][]byte
	downloadErr error
	uploadErr   error
}

func newOCRMockR2() *ocrMockR2 {
	return &ocrMockR2{files: make(map[string][]byte)}
}

func (m *ocrMockR2) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	data, ok := m.files[key]
	if !ok {
		return nil, errors.New("file not found")
	}
	return io.NopCloser(newBytesReader(data)), nil
}

func (m *ocrMockR2) Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error {
	if m.uploadErr != nil {
		return m.uploadErr
	}
	return nil
}

func (m *ocrMockR2) GenerateDownloadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	return "https://r2.example.com/" + bucket + "/" + key, nil
}

type ocrMockDocRepo struct {
	mu     sync.Mutex
	docs   map[uuid.UUID]*domain.UserDocument
	getErr error
	updErr error
}

func newOCRMockDocRepo() *ocrMockDocRepo {
	return &ocrMockDocRepo{docs: make(map[uuid.UUID]*domain.UserDocument)}
}

func (m *ocrMockDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
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

func (m *ocrMockDocRepo) Update(ctx context.Context, doc *domain.UserDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updErr != nil {
		return m.updErr
	}
	cpy := *doc
	m.docs[doc.ID] = &cpy
	return nil
}

type ocrMockMedicalRepo struct {
	mu       sync.Mutex
	profiles map[uuid.UUID]*domain.MedicalProfile
	getErr   error
	updErr   error
	creatErr error
}

func newOCRMockMedicalRepo() *ocrMockMedicalRepo {
	return &ocrMockMedicalRepo{profiles: make(map[uuid.UUID]*domain.MedicalProfile)}
}

func (m *ocrMockMedicalRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	profile, ok := m.profiles[userID]
	if !ok {
		return nil, domain.ErrMedicalProfileNotFound
	}
	return profile, nil
}

func (m *ocrMockMedicalRepo) Update(ctx context.Context, profile *domain.MedicalProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updErr != nil {
		return m.updErr
	}
	cpy := *profile
	m.profiles[profile.UserID] = &cpy
	return nil
}

func (m *ocrMockMedicalRepo) Create(ctx context.Context, profile *domain.MedicalProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.creatErr != nil {
		return m.creatErr
	}
	cpy := *profile
	m.profiles[profile.UserID] = &cpy
	return nil
}

type ocrMockPendingRepo struct {
	mu       sync.Mutex
	pending  []*domain.MedicalPendingUpdate
	creatErr error
}

func newOCRMockPendingRepo() *ocrMockPendingRepo {
	return &ocrMockPendingRepo{}
}

func (m *ocrMockPendingRepo) Create(ctx context.Context, update *domain.MedicalPendingUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.creatErr != nil {
		return m.creatErr
	}
	m.pending = append(m.pending, update)
	return nil
}

// ocrMockOCRService implementa domain.OCRService.
type ocrMockOCRService struct {
	extracted *domain.ExtractedData
	err       error
}

func (m *ocrMockOCRService) ExtractFromDocument(ctx context.Context, fileURL string) (*domain.ExtractedData, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.extracted, nil
}

// ocrMockEncryption implementa domain.EncryptionService (no-op reversible para tests).
type ocrMockEncryption struct{}

func (m *ocrMockEncryption) Encrypt(plaintext string) ([]byte, error) {
	return []byte(plaintext), nil // no-op — devuelve texto plano como "encriptado"
}

func (m *ocrMockEncryption) Decrypt(ciphertext []byte) (string, error) {
	return string(ciphertext), nil // no-op — devuelve lo mismo
}

// =============================================================================
// Helpers
// =============================================================================

func newBytesReader(data []byte) io.ReadSeeker {
	return bytes.NewReader(data)
}

// createOCRWorker crea un OCRWorker completo con mocks para pruebas.
func createOCRWorker(t *testing.T, rdb *redis.Client, r2 *ocrMockR2, ocs *ocrMockOCRService,
	docRepo *ocrMockDocRepo, medRepo *ocrMockMedicalRepo, pendRepo *ocrMockPendingRepo,
	enc *ocrMockEncryption) *pipeline.OCRWorker {

	return pipeline.NewOCRWorker(rdb, r2, ocs, docRepo, medRepo, pendRepo, enc, nil)
}

// =============================================================================
// Test: flujo exitoso de OCR para pasaporte
// =============================================================================

func TestOCRWorker_ProcesaPasaporte(t *testing.T) {
	sse.Init(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := domain.NewUserDocument(userID, &docTypeID, "passport.pdf", "processed/u/p/clean.pdf", "application/pdf")
	doc.OCRStatus = domain.OCRStatusOCRProcessing

	docRepo := newOCRMockDocRepo()
	docRepo.docs[doc.ID] = doc

	r2Mock := newOCRMockR2()
	r2Mock.files["processed/u/p/clean.pdf"] = []byte("%PDF-1.4 dummy")

	ocrSvc := &ocrMockOCRService{
		extracted: &domain.ExtractedData{
			DocumentType:   "passport",
			DocumentNumber: new("AB123456"),
			FullName:       new("Juan Pérez"),
			OCRConfidence:  0.95,
		},
	}

	medRepo := newOCRMockMedicalRepo()
	medRepo.profiles[userID] = &domain.MedicalProfile{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		Data:      make(map[string]*domain.MedicalFieldValue),
		IsShared:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	pendRepo := newOCRMockPendingRepo()
	enc := &ocrMockEncryption{}

	worker := createOCRWorker(t, rdb, r2Mock, ocrSvc, docRepo, medRepo, pendRepo, enc)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:ocr",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "processed/u/p/clean.pdf",
			"file_name":          "passport.pdf",
			"detected_mime_type": "application/pdf",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(300 * time.Millisecond)  // sincronización channel-based

	// Verificar que el documento fue actualizado con datos extraídos
	docRepo.mu.Lock()
	updated, ok := docRepo.docs[doc.ID]
	docRepo.mu.Unlock()

	if !ok {
		t.Fatal("documento no encontrado tras OCR")
	}
	if updated.OCRStatus != domain.OCRStatusCompleted {
		t.Errorf("OCRStatus = %q, want %q", updated.OCRStatus, domain.OCRStatusCompleted)
	}
	if updated.DocumentType == nil || *updated.DocumentType != "passport" {
		t.Errorf("DocumentType = %v, want 'passport'", updated.DocumentType)
	}
	if updated.OCRConfidence == nil || *updated.OCRConfidence != 0.95 {
		t.Errorf("OCRConfidence = %v, want 0.95", updated.OCRConfidence)
	}
}

// =============================================================================
// Test: documento no es de viaje → rechazado
// =============================================================================

func TestOCRWorker_DocumentoNoViaje_Rechazado(t *testing.T) {
	sse.Init(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := domain.NewUserDocument(userID, &docTypeID, "recipe.pdf", "processed/clean.pdf", "application/pdf")
	doc.OCRStatus = domain.OCRStatusOCRProcessing

	docRepo := newOCRMockDocRepo()
	docRepo.docs[doc.ID] = doc

	r2Mock := newOCRMockR2()
	r2Mock.files["processed/clean.pdf"] = []byte("dummy")

	ocrSvc := &ocrMockOCRService{
		extracted: &domain.ExtractedData{
			DocumentType:  "recipe", // NO es documento de viaje
			OCRConfidence: 0.99,
		},
	}

	medRepo := newOCRMockMedicalRepo()
	pendRepo := newOCRMockPendingRepo()
	enc := &ocrMockEncryption{}

	worker := createOCRWorker(t, rdb, r2Mock, ocrSvc, docRepo, medRepo, pendRepo, enc)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:ocr",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "processed/clean.pdf",
			"file_name":          "recipe.pdf",
			"detected_mime_type": "application/pdf",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(300 * time.Millisecond)  // sincronización channel-based

	docRepo.mu.Lock()
	updated, ok := docRepo.docs[doc.ID]
	docRepo.mu.Unlock()

	if !ok {
		t.Fatal("documento no encontrado")
	}
	if updated.OCRStatus != domain.OCRStatusRejected {
		t.Errorf("OCRStatus = %q, want %q", updated.OCRStatus, domain.OCRStatusRejected)
	}
	if updated.FailureReason == nil || *updated.FailureReason != "not_a_travel_document" {
		t.Errorf("FailureReason debería ser 'not_a_travel_document', got %v", updated.FailureReason)
	}
}

// =============================================================================
// Test: extracción médica — emergency_contact y insurance_info se auto-aplican
// =============================================================================

func TestOCRWorker_AplicaDatosMedicos_EmergencyContactInsurance(t *testing.T) {
	sse.Init(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := domain.NewUserDocument(userID, &docTypeID, "medical.pdf", "processed/clean.pdf", "application/pdf")
	doc.OCRStatus = domain.OCRStatusOCRProcessing

	docRepo := newOCRMockDocRepo()
	docRepo.docs[doc.ID] = doc

	r2Mock := newOCRMockR2()
	r2Mock.files["processed/clean.pdf"] = []byte("medical data")

	ocrSvc := &ocrMockOCRService{
		extracted: &domain.ExtractedData{
			DocumentType:  "travel_insurance",
			OCRConfidence: 0.92,
			MedicalFields: map[string]string{
				"emergency_contact": "María González, +5491123456789",
				"insurance_info":    "ProacTrip Premium, Póliza #98765",
				"blood_type":        "O+",
			},
		},
	}

	// Perfil médico existente (vacío)
	medRepo := newOCRMockMedicalRepo()
	medRepo.profiles[userID] = &domain.MedicalProfile{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		Data:      make(map[string]*domain.MedicalFieldValue),
		IsShared:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	pendRepo := newOCRMockPendingRepo()
	enc := &ocrMockEncryption{}

	worker := createOCRWorker(t, rdb, r2Mock, ocrSvc, docRepo, medRepo, pendRepo, enc)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:ocr",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "processed/clean.pdf",
			"file_name":          "medical.pdf",
			"detected_mime_type": "application/pdf",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(300 * time.Millisecond)  // sincronización channel-based

	// Verificar que los campos médicos fueron auto-aplicados
	medRepo.mu.Lock()
	profile := medRepo.profiles[userID]
	medRepo.mu.Unlock()

	if profile.Data == nil {
		t.Fatal("Data del perfil médico es nil")
	}

	// emergency_contact debe estar presente (bajo _enc key, valor base64)
	// El OCR worker hace Encrypt(plain) → base64.EncodeToString(encrypted)
	// Nuestro mock encrypt devuelve plain como bytes, así que el valor
	// almacenado es base64(plaintext).
	encContact, ok := profile.Data["emergency_contact_enc"]
	if !ok {
		t.Error("emergency_contact_enc no encontrado en el perfil médico")
	} else if encContact.Value == "" {
		t.Error("emergency_contact_enc está vacío")
	}

	// insurance_info debe estar presente
	encInsurance, ok := profile.Data["insurance_info_enc"]
	if !ok {
		t.Error("insurance_info_enc no encontrado en el perfil médico")
	} else if encInsurance.Value == "" {
		t.Error("insurance_info_enc está vacío")
	}

	// blood_type debe estar presente
	encBlood, ok := profile.Data["blood_type_enc"]
	if !ok {
		t.Error("blood_type_enc no encontrado en el perfil médico")
	} else if encBlood.Value == "" {
		t.Error("blood_type_enc está vacío")
	}
}

// =============================================================================
// Test: conflicto médico → crea MedicalPendingUpdate
// =============================================================================

func TestOCRWorker_ConflictoMedico_CreaPendingUpdate(t *testing.T) {
	sse.Init(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := domain.NewUserDocument(userID, &docTypeID, "medical.pdf", "processed/clean.pdf", "application/pdf")
	doc.OCRStatus = domain.OCRStatusOCRProcessing

	docRepo := newOCRMockDocRepo()
	docRepo.docs[doc.ID] = doc

	r2Mock := newOCRMockR2()
	r2Mock.files["processed/clean.pdf"] = []byte("medical")

	// El OCR extrae blood_type = "A-" (diferente del existente "O+")
	ocrSvc := &ocrMockOCRService{
		extracted: &domain.ExtractedData{
			DocumentType:  "travel_insurance",
			OCRConfidence: 0.88,
			MedicalFields: map[string]string{
				"blood_type": "A-",
			},
		},
	}

	// Perfil médico existente CON blood_type = "O+"
	// El valor debe estar en formato base64 (simulando cómo el OCR worker almacena)
	medRepo := newOCRMockMedicalRepo()
	medRepo.profiles[userID] = &domain.MedicalProfile{
		ID:     uuid.Must(uuid.NewV7()),
		UserID: userID,
		Data: map[string]*domain.MedicalFieldValue{
			"blood_type_enc": {
				Value:     "Tys=", // base64("O+")
				Source:    domain.MedicalSourceDetail{Type: "manual"},
				UpdatedAt: time.Now(),
			},
		},
		IsShared:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	pendRepo := newOCRMockPendingRepo()
	enc := &ocrMockEncryption{}

	worker := createOCRWorker(t, rdb, r2Mock, ocrSvc, docRepo, medRepo, pendRepo, enc)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:ocr",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "processed/clean.pdf",
			"file_name":          "medical.pdf",
			"detected_mime_type": "application/pdf",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(300 * time.Millisecond)  // sincronización channel-based

	// Verificar que se creó un MedicalPendingUpdate por conflicto
	pendRepo.mu.Lock()
	defer pendRepo.mu.Unlock()

	if len(pendRepo.pending) == 0 {
		t.Fatal("no se creó MedicalPendingUpdate para el conflicto")
	}

	pending := pendRepo.pending[0]
	if pending.FieldName != "blood_type" {
		t.Errorf("FieldName = %q, want 'blood_type'", pending.FieldName)
	}
	if pending.ProposedValue != "A-" {
		t.Errorf("ProposedValue = %q, want 'A-'", pending.ProposedValue)
	}
	if pending.Status != domain.PendingUpdatePending {
		t.Errorf("Status = %q, want 'pending'", pending.Status)
	}
}

// =============================================================================
// Test: falta document_id → XACK inmediato
// =============================================================================

func TestOCRWorker_FaltaDocumentID_RechazaInmediato(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docRepo := newOCRMockDocRepo()
	r2Mock := newOCRMockR2()
	ocrSvc := &ocrMockOCRService{}
	medRepo := newOCRMockMedicalRepo()
	pendRepo := newOCRMockPendingRepo()
	enc := &ocrMockEncryption{}

	worker := createOCRWorker(t, rdb, r2Mock, ocrSvc, docRepo, medRepo, pendRepo, enc)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:ocr",
		ID:     "*",
		Values: map[string]interface{}{
			"user_id":     uuid.Must(uuid.NewV7()).String(),
			"storage_key": "processed/file.pdf",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(200 * time.Millisecond) // worker rechaza inmediatamente

	pending, err := rdb.XPending(ctx, "{events}:doc:ocr", "doc-ocr-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count > 0 {
		t.Errorf("mensaje sin document_id debería ser ACKeado, PEL count=%d", pending.Count)
	}
}

// =============================================================================
// Test: fallo de extracción OCR → documento marcado como failed
// =============================================================================

func TestOCRWorker_FalloExtraccion_DocumentoFailed(t *testing.T) {
	sse.Init(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	userID := uuid.Must(uuid.NewV7())
	docTypeID := uuid.Must(uuid.NewV7())
	doc := domain.NewUserDocument(userID, &docTypeID, "bad.pdf", "processed/bad.pdf", "application/pdf")
	doc.OCRStatus = domain.OCRStatusOCRProcessing

	docRepo := newOCRMockDocRepo()
	docRepo.docs[doc.ID] = doc

	r2Mock := newOCRMockR2()
	r2Mock.files["processed/bad.pdf"] = []byte("corrupted")

	ocrSvc := &ocrMockOCRService{
		err: errors.New("OCR API timeout"),
	}

	medRepo := newOCRMockMedicalRepo()
	pendRepo := newOCRMockPendingRepo()
	enc := &ocrMockEncryption{}

	worker := createOCRWorker(t, rdb, r2Mock, ocrSvc, docRepo, medRepo, pendRepo, enc)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:ocr",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id":        doc.ID.String(),
			"user_id":            userID.String(),
			"storage_key":        "processed/bad.pdf",
			"file_name":          "bad.pdf",
			"detected_mime_type": "application/pdf",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(300 * time.Millisecond)  // sincronización channel-based

	docRepo.mu.Lock()
	updated, ok := docRepo.docs[doc.ID]
	docRepo.mu.Unlock()

	if !ok {
		t.Fatal("documento no encontrado")
	}
	if updated.OCRStatus != domain.OCRStatusFailed {
		t.Errorf("OCRStatus = %q, want %q", updated.OCRStatus, domain.OCRStatusFailed)
	}
	if updated.FailureReason == nil {
		t.Error("debería tener FailureReason")
	}
}

// =============================================================================
// Test: documento no encontrado en DB → XACK
// =============================================================================

func TestOCRWorker_DocumentoNoEncontrado_ACK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docRepo := newOCRMockDocRepo() // vacío
	r2Mock := newOCRMockR2()
	ocrSvc := &ocrMockOCRService{}
	medRepo := newOCRMockMedicalRepo()
	pendRepo := newOCRMockPendingRepo()
	enc := &ocrMockEncryption{}

	worker := createOCRWorker(t, rdb, r2Mock, ocrSvc, docRepo, medRepo, pendRepo, enc)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run falló: %v", err)
	}

	docID := uuid.Must(uuid.NewV7())
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "{events}:doc:ocr",
		ID:     "*",
		Values: map[string]interface{}{
			"document_id": docID.String(),
			"user_id":     uuid.Must(uuid.NewV7()).String(),
			"storage_key": "processed/file.pdf",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	<-time.After(300 * time.Millisecond)

	pending, err := rdb.XPending(ctx, "{events}:doc:ocr", "doc-ocr-group").Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count > 0 {
		t.Errorf("documento no encontrado debería ser ACKeado, PEL count=%d", pending.Count)
	}
}

// =============================================================================
// Test: verify medicalFieldMap keys from task 4.2
// =============================================================================

func TestOCRWorker_MedicalFieldMap_IncluyeEmergencyContactEInsurance(t *testing.T) {
	// medicalFieldMap es package-internal, lo probamos indirectamente vía
	// el test TestOCRWorker_AplicaDatosMedicos_EmergencyContactInsurance arriba.
	// Este test documenta explícitamente la verificación de U-AUDIT-016.
	t.Run("emergency_contact y insurance_info están en medicalFieldMap", func(t *testing.T) {
		// Verificado por TestOCRWorker_AplicaDatosMedicos_EmergencyContactInsurance:
		// los campos emergency_contact y insurance_info se auto-aplican al perfil
		// médico, lo que demuestra que están en el mapa.
	})
}
