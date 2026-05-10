package download_document

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

type testDDocRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}
func (m *testDDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil { return m.getByIDFn(ctx, id) }; return nil, nil
}
type testDStorage struct {
	downloadFn func(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}
func (m *testDStorage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if m.downloadFn != nil { return m.downloadFn(ctx, bucket, key) }; return io.NopCloser(strings.NewReader("test")), nil
}

func TestDownloadDocumentHandler_Handle(t *testing.T) {
	uid, docID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	tc := &sharedauth.AccessClaims{UserID: uid, Email: "t@t.com", RoleID: uuid.Must(uuid.NewV7()), Role: "client", SessionID: uuid.Must(uuid.NewV7()), JTI: uuid.Must(uuid.NewV7())}
	t.Run("debe streamear 200 con archivo", func(t *testing.T) {
		e := echo.New(); req := httptest.NewRequest(http.MethodGet, "/v1/user/documents/"+docID.String()+"/download", nil)
		rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
		c.SetPath("/v1/user/documents/:document_id/download"); c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		c.Set("user_claims", tc)
		uc := NewUseCase(UseCaseDeps{
			DocRepo: &testDDocRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
				mime := "application/pdf"
				return &domain.UserDocument{ID: docID, UserID: uid, FileName: "doc.pdf", MimeType: &mime, OCRStatus: domain.OCRStatusCompleted, StorageKey: "raw/key"}, nil
			}},
			Storage: &testDStorage{},
		}); h := NewHandler(uc); _ = h.Handle(c)
		if rec.Code != http.StatusOK { t.Errorf("status = %d, want 200", rec.Code) }
	})
	t.Run("debe retornar error sin claims", func(t *testing.T) {
		e := echo.New(); req := httptest.NewRequest(http.MethodGet, "/v1/user/documents/"+docID.String()+"/download", nil)
		rec := httptest.NewRecorder(); c := e.NewContext(req, rec)
		c.SetPath("/v1/user/documents/:document_id/download"); c.SetPathValues(echo.PathValues{{Name: "document_id", Value: docID.String()}})
		uc := NewUseCase(UseCaseDeps{DocRepo: &testDDocRepo{}, Storage: &testDStorage{}}); h := NewHandler(uc); _ = h.Handle(c)
		if rec.Code == http.StatusOK { t.Errorf("status = %d, expected error", rec.Code) }
	})
}
