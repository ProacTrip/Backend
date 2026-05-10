// Tests para el usecase download_document.
// Verifica ownership, OCR status, y descarga de R2.
package download_document

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockDocRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error)
}

func (m *mockDocRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

type mockStorage struct {
	downloadFn func(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

func (m *mockStorage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if m.downloadFn != nil {
		return m.downloadFn(ctx, bucket, key)
	}
	return nil, nil
}

// =============================================================================
// Helpers
// =============================================================================

func testDoc(userID, docID uuid.UUID, status domain.OCRStatus) *domain.UserDocument {
	mime := "application/pdf"
	return &domain.UserDocument{
		ID:        docID,
		UserID:    userID,
		FileName:  "documento.pdf",
		MimeType:  &mime,
		OCRStatus: status,
		StorageKey: "raw/" + userID.String() + "/" + docID.String(),
	}
}

// =============================================================================
// Tests — Table-driven
// =============================================================================

func TestDownloadDocumentUseCase_Execute(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())
	otherUserID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name     string
		cmd      Command
		docRepo  *mockDocRepo
		storage  *mockStorage
		wantErr  bool
		wantErrIs error
	}{
		{
			name: "debe retornar documento encontrado con status completed",
			cmd:  Command{DocumentID: docID, UserID: userID},
			docRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return testDoc(userID, docID, domain.OCRStatusCompleted), nil
				},
			},
			storage: &mockStorage{
				downloadFn: func(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("contenido")), nil
				},
			},
			wantErr: false,
		},
		{
			name: "debe retornar documento encontrado con status rejected (también permitido)",
			cmd:  Command{DocumentID: docID, UserID: userID},
			docRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return testDoc(userID, docID, domain.OCRStatusRejected), nil
				},
			},
			storage: &mockStorage{
				downloadFn: func(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("contenido rechazado")), nil
				},
			},
			wantErr: false,
		},
		{
			name: "debe retornar error cuando documento no existe",
			cmd:  Command{DocumentID: docID, UserID: userID},
			docRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return nil, domain.ErrDocumentNotFound
				},
			},
			storage: &mockStorage{},
			wantErr: true,
		},
		{
			name: "debe retornar ErrDocumentNotFound cuando usuario no es dueño del documento",
			cmd:  Command{DocumentID: docID, UserID: userID},
			docRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return testDoc(otherUserID, docID, domain.OCRStatusCompleted), nil
				},
			},
			storage: &mockStorage{},
			wantErr: true,
			wantErrIs: domain.ErrDocumentNotFound,
		},
		{
			name: "debe retornar ErrDocumentNotReady cuando OCR status no es completed ni rejected",
			cmd:  Command{DocumentID: docID, UserID: userID},
			docRepo: &mockDocRepo{
				getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserDocument, error) {
					return testDoc(userID, docID, domain.OCRStatusQueued), nil
				},
			},
			storage: &mockStorage{},
			wantErr: true,
			wantErrIs: domain.ErrDocumentNotReady,
		},
		{
			name: "debe retornar error cuando DocumentID es uuid.Nil",
			cmd:  Command{DocumentID: uuid.Nil, UserID: userID},
			docRepo: &mockDocRepo{},
			storage: &mockStorage{},
			wantErr: true,
		},
		{
			name: "debe retornar error cuando UserID es uuid.Nil",
			cmd:  Command{DocumentID: docID, UserID: uuid.Nil},
			docRepo: &mockDocRepo{},
			storage: &mockStorage{},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			uc := NewUseCase(UseCaseDeps{
				DocRepo: tc.docRepo,
				Storage: tc.storage,
			})

			resp, err := uc.Execute(ctx, tc.cmd)

			if tc.wantErr && err == nil {
				t.Error("se esperaba error, obtuve nil")
				return
			}
			if !tc.wantErr && err != nil {
				t.Errorf("error inesperado: %v", err)
				return
			}

			if !tc.wantErr {
				if resp == nil {
					t.Fatal("respuesta no debería ser nil")
				}
				defer resp.Reader.Close()
				if resp.FileName != "documento.pdf" {
					t.Errorf("FileName = %s, se esperaba documento.pdf", resp.FileName)
				}
				if resp.MimeType != "application/pdf" {
					t.Errorf("MimeType = %s, se esperaba application/pdf", resp.MimeType)
				}
			}
		})
	}
}
