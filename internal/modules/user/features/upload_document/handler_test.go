// Tests para el handler POST /v1/user/documents.
// Prueba extracción de claims y delegación al usecase.
// El flujo completo requiere miniredis — los tests de integración lo cubren.
package upload_document

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type testUDDocRepo struct {
	createFn      func(ctx context.Context, doc *domain.UserDocument) error
	getByIDFn     func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
	countByUserIDFn func(ctx context.Context, userID uuid.UUID) (int, error)
	deleteFn      func(ctx context.Context, id uuid.UUID) error
}

func (m *testUDDocRepo) Create(ctx context.Context, doc *domain.UserDocument) error {
	if m.createFn != nil {
		return m.createFn(ctx, doc)
	}
	return nil
}
func (m *testUDDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *testUDDocRepo) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	if m.countByUserIDFn != nil {
		return m.countByUserIDFn(ctx, userID)
	}
	return 0, nil
}
func (m *testUDDocRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

type testUDStorage struct {
	uploadFn   func(ctx context.Context, bucket, key string, reader io.Reader, size int64, ct string) error
	downloadFn func(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	deleteFn   func(ctx context.Context, bucket, key string) error
}

func (m *testUDStorage) Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, ct string) error {
	if m.uploadFn != nil {
		return m.uploadFn(ctx, bucket, key, reader, size, ct)
	}
	return nil
}
func (m *testUDStorage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	return nil, nil
}
func (m *testUDStorage) Delete(ctx context.Context, bucket, key string) error {
	return nil
}

type testUDEventPub struct {
	publishFn func(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

func (m *testUDEventPub) Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error) {
	if m.publishFn != nil {
		return m.publishFn(ctx, stream, payload)
	}
	return "event-id", nil
}

// =============================================================================
// Helpers
// =============================================================================

func testClaims() *sharedauth.AccessClaims {
	return &sharedauth.AccessClaims{
		UserID:    uuid.Must(uuid.NewV7()),
		Email:     "test@example.com",
		RoleID:    uuid.Must(uuid.NewV7()),
		Role:      "client",
		JTI: uuid.Must(uuid.NewV7()),
	}
}

func newMultipartUploadReq(fileBytes []byte, fileName string) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(fileBytes); err != nil {
		return nil, err
	}
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/user/documents", body)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body.Bytes())))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestUploadDocumentHandler_Handle(t *testing.T) {
	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}

	t.Run("debe retornar error cuando no hay claims", func(t *testing.T) {
		req, err := newMultipartUploadReq(pngBytes, "test.png")
		if err != nil {
			t.Fatalf("error creando request: %v", err)
		}
		e := echo.New()
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		uc := NewUseCase(UseCaseDeps{
			DocRepo:        &testUDDocRepo{},
			Storage:        &testUDStorage{},
			EventPublisher: &testUDEventPub{},
			Dragonfly:      nil,
		})
		h := NewHandler(uc)
		_ = h.Handle(c)

		if rec.Code == http.StatusOK || rec.Code == http.StatusAccepted {
			t.Errorf("status = %d, se esperaba error cuando no hay claims", rec.Code)
		}
	})

	_ = redis.NewClient(&redis.Options{})
}
