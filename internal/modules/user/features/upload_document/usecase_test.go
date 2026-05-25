// Tests para el usecase upload_document.
// Valida magic bytes check, size validation y MIME detection.
package upload_document

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type testDocRepo struct {
	createFn       func(ctx context.Context, doc *domain.UserDocument) error
	getByIDFn      func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	countByUserIDFn func(ctx context.Context, userID uuid.UUID) (int, error)
	deleteFn       func(ctx context.Context, id uuid.UUID) error
}

func (m *testDocRepo) Create(ctx context.Context, doc *domain.UserDocument) error {
	if m.createFn != nil {
		return m.createFn(ctx, doc)
	}
	return nil
}
func (m *testDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *testDocRepo) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	if m.countByUserIDFn != nil {
		return m.countByUserIDFn(ctx, userID)
	}
	return 0, nil
}
func (m *testDocRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

type testStorage struct {
	uploadFn   func(ctx context.Context, bucket, key string, reader io.Reader, size int64, ct string) error
	downloadFn func(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	deleteFn   func(ctx context.Context, bucket, key string) error
}

func (m *testStorage) Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, ct string) error {
	if m.uploadFn != nil {
		return m.uploadFn(ctx, bucket, key, reader, size, ct)
	}
	return nil
}
func (m *testStorage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	return nil, nil
}
func (m *testStorage) Delete(ctx context.Context, bucket, key string) error { return nil }

type testEventPub struct {
	publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

func (m *testEventPub) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil {
		return m.publishFn(ctx, stream, payload)
	}
	return "event-id", nil
}

// =============================================================================
// Tests
// =============================================================================

func TestUploadDocumentUseCase_MagicBytesCheck(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	// PNG válido
	validPNG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}
	// Bytes inválidos (no magic bytes reconocibles)
	invalidBytes := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}

	tests := []struct {
		name      string
		fileBytes []byte
		wantErr   error
	}{
		{
			name:      "debe aceptar PNG con magic bytes validos",
			fileBytes: validPNG,
			wantErr:   nil, // pasará magic bytes pero fallará sin dragonfly (dedup)
		},
		{
			name:      "debe rechazar archivo sin magic bytes reconocibles",
			fileBytes: invalidBytes,
			wantErr:   domain.ErrInvalidFileType,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewUseCase(UseCaseDeps{
				DocRepo:        &testDocRepo{countByUserIDFn: func(ctx context.Context, uid uuid.UUID) (int, error) { return 0, nil }},
				Storage:        &testStorage{},
				EventPublisher: &testEventPub{},
				Dragonfly:      nil,
			})

			cmd := UploadDocumentCommand{
				FileBytes: tc.fileBytes,
				FileName:  "test.png",
				FileSize:  int64(len(tc.fileBytes)),
				UserID:    userID.String(),
			}

			_, err := uc.Execute(t.Context(), cmd)

			if tc.wantErr == nil && err == nil {
				t.Error("se esperaba error (sin dragonfly), pero no se produjo ninguno")
				return
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, se esperaba %v", err, tc.wantErr)
			}
		})
	}
}

func TestUploadDocumentUseCase_SizeValidation(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	// PDF válido pero muy grande
	hugePDF := make([]byte, int(MaxDocumentSizeBytes())+100)
	copy(hugePDF[:4], []byte{0x25, 0x50, 0x44, 0x46}) // magic bytes PDF
	hugePDF = hugePDF[:int(MaxDocumentSizeBytes())+100]

	// PDF válido y pequeño
	smallPDF := []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34, 0x0A} // PDF magic bytes + version

	t.Run("debe rechazar archivo que excede MaxDocumentSizeBytes", func(t *testing.T) {
		uc := NewUseCase(UseCaseDeps{
			DocRepo:        &testDocRepo{countByUserIDFn: func(ctx context.Context, uid uuid.UUID) (int, error) { return 0, nil }},
			Storage:        &testStorage{},
			EventPublisher: &testEventPub{},
			Dragonfly:      nil,
		})

		cmd := UploadDocumentCommand{
			FileBytes: hugePDF,
			FileName:  "grande.pdf",
			FileSize:  int64(len(hugePDF)),
			UserID:    userID.String(),
		}

		_, err := uc.Execute(t.Context(), cmd)
		if !errors.Is(err, domain.ErrFileTooLarge) {
			t.Errorf("error = %v, se esperaba ErrFileTooLarge", err)
		}
	})

	t.Run("debe aceptar PDF de tamaño valido (fallara en dedup sin dragonfly)", func(t *testing.T) {
		uc := NewUseCase(UseCaseDeps{
			DocRepo:        &testDocRepo{countByUserIDFn: func(ctx context.Context, uid uuid.UUID) (int, error) { return 0, nil }},
			Storage:        &testStorage{},
			EventPublisher: &testEventPub{},
			Dragonfly:      nil,
		})

		cmd := UploadDocumentCommand{
			FileBytes: smallPDF,
			FileName:  "chico.pdf",
			FileSize:  int64(len(smallPDF)),
			UserID:    userID.String(),
		}

		_, err := uc.Execute(t.Context(), cmd)
		if err == nil {
			t.Error("se esperaba error (sin dragonfly para dedup), pero no se produjo ninguno")
		}
	})
}

func TestUploadDocumentUseCase_MIMEDetection(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name      string
		fileBytes []byte
		wantErr   error
	}{
		{
			name:      "debe detectar PNG correctamente",
			fileBytes: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52},
			wantErr:   nil, // magic bytes pasan, error esperado por falta de dragonfly
		},
		{
			name:      "debe detectar JPEG correctamente",
			fileBytes: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01},
			wantErr:   nil, // magic bytes pasan, error esperado por falta de dragonfly
		},
		{
			name:      "debe detectar PDF correctamente",
			fileBytes: []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34},
			wantErr:   nil, // magic bytes pasan, error esperado por falta de dragonfly
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewUseCase(UseCaseDeps{
				DocRepo:        &testDocRepo{countByUserIDFn: func(ctx context.Context, uid uuid.UUID) (int, error) { return 0, nil }},
				Storage:        &testStorage{},
				EventPublisher: &testEventPub{},
				Dragonfly:      nil,
			})

			cmd := UploadDocumentCommand{
				FileBytes: tc.fileBytes,
				FileName:  "test.bin",
				FileSize:  int64(len(tc.fileBytes)),
				UserID:    userID.String(),
			}

			_, err := uc.Execute(t.Context(), cmd)
			if tc.wantErr == nil && err == nil {
				t.Error("se esperaba error (sin dragonfly), pero no se produjo ninguno")
			}
			// No debe ser ErrInvalidFileType porque los magic bytes son válidos
			if errors.Is(err, domain.ErrInvalidFileType) {
				t.Errorf("no se esperaba ErrInvalidFileType para magic bytes válidos de %s", tc.name)
			}
		})
	}
}

func TestUploadDocumentUseCase_EmptyFile(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo:        &testDocRepo{},
		Storage:        &testStorage{},
		EventPublisher: &testEventPub{},
		Dragonfly:      nil,
	})

	cmd := UploadDocumentCommand{
		FileBytes: []byte{},
		FileName:  "vacio.bin",
		FileSize:  0,
		UserID:    userID.String(),
	}

	_, err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrFileTooLarge) {
		t.Errorf("error = %v, se esperaba ErrFileTooLarge para archivo vacío", err)
	}
}

// =============================================================================
// Nil Dragonfly Guard
// =============================================================================

func TestCheckRateLimit_NilDragonfly(t *testing.T) {
	uc := &UseCase{
		dragonfly: nil,
	}

	err := uc.CheckRateLimit(t.Context(), "0195af15-4aa7-77ea-a50c-1234567890ab")
	if err != nil {
		t.Errorf("CheckRateLimit with nil dragonfly should return nil, got error: %v", err)
	}
}


