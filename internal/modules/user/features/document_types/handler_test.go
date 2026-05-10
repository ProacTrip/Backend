// Tests para el handler GET /v1/user/documents/types.
package document_types

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type testTypeRepo struct{ getTypesFn func(ctx context.Context) ([]domain.DocumentType, error) }

func (m *testTypeRepo) GetTypes(ctx context.Context) ([]domain.DocumentType, error) {
	if m.getTypesFn != nil {
		return m.getTypesFn(ctx)
	}
	return nil, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestDocumentTypesHandler_Handle(t *testing.T) {
	tests := []struct {
		name       string
		repo       *testTypeRepo
		wantStatus int
	}{
		{
			name: "debe retornar 200 con catalogo",
			repo: &testTypeRepo{getTypesFn: func(ctx context.Context) ([]domain.DocumentType, error) {
				return []domain.DocumentType{{ID: uuid.Nil, Code: "passport", Name: "Pasaporte", IsActive: true}}, nil
			}},
			wantStatus: http.StatusOK,
		},
		{
			name: "debe retornar error cuando repo falla",
			repo: &testTypeRepo{getTypesFn: func(ctx context.Context) ([]domain.DocumentType, error) {
				return nil, domain.ErrDocumentNotFound
			}},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/v1/user/documents/types", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			uc := NewUseCase(UseCaseDeps{TypeRepo: tt.repo})
			h := NewHandler(uc)
			_ = h.Handle(c)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
