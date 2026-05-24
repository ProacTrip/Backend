// Tests para el usecase list_documents.
// Valida listado con/sin filtros y lista vacía.
package list_documents

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type testDocRepo struct {
	getByUserIDFn         func(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error)
	getByUserIDFilteredFn func(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error)
}

func (m *testDocRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.UserDocument, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *testDocRepo) GetByUserIDFiltered(ctx context.Context, userID uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error) {
	if m.getByUserIDFilteredFn != nil {
		return m.getByUserIDFilteredFn(ctx, userID, status, docType)
	}
	return nil, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestListDocumentsUseCase_WithoutFilters(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mime := "image/png"
	size := 512

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
				return []*domain.UserDocument{
					{
						ID:        uuid.Must(uuid.NewV7()),
						UserID:    userID,
						FileName:  "foto.png",
						MimeType:  &mime,
						FileSize:  &size,
						OCRStatus: domain.OCRStatusCompleted,
					},
					{
						ID:        uuid.Must(uuid.NewV7()),
						UserID:    userID,
						FileName:  "doc.pdf",
						MimeType:  new(string),
						FileSize:  &size,
						OCRStatus: domain.OCRStatusQueued,
					},
				}, nil
			},
		},
	})

	docs, err := uc.Execute(t.Context(), userID.String(), "", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("len(docs) = %d, se esperaba 2", len(docs))
	}
}

func TestListDocumentsUseCase_WithFilters(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	filteredCalled := false
	unfilteredCalled := false

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
				unfilteredCalled = true
				return []*domain.UserDocument{}, nil
			},
			getByUserIDFilteredFn: func(ctx context.Context, uid uuid.UUID, status domain.OCRStatus, docType string) ([]*domain.UserDocument, error) {
				filteredCalled = true
				return []*domain.UserDocument{}, nil
			},
		},
	})

	// Con filtro status
	_, err := uc.Execute(t.Context(), userID.String(), "completed", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !filteredCalled {
		t.Error("GetByUserIDFiltered debería ser llamado cuando hay filtro")
	}
	if unfilteredCalled {
		t.Error("GetByUserID no debería ser llamado cuando hay filtro")
	}
}

func TestListDocumentsUseCase_EmptyList(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		DocRepo: &testDocRepo{
			getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
				return nil, nil
			},
		},
	})

	docs, err := uc.Execute(t.Context(), userID.String(), "", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if docs == nil {
		t.Error("docs no debería ser nil, debería ser slice vacío")
	}
	if len(docs) != 0 {
		t.Errorf("len(docs) = %d, se esperaba 0", len(docs))
	}
}

// =============================================================================
// T-4.4: Filter validation — status/document_type enum
// =============================================================================

func TestListDocumentsUseCase_StatusFilterValidation(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name       string
		status     string
		expectErr  bool
	}{
		{"completed válido", "completed", false},
		{"queued válido", "queued", false},
		{"processing válido", "processing", false},
		{"rejected válido", "rejected", false},
		{"failed válido", "failed", false},
		{"invalid inválido", "invalid", true},
		{"pending inválido", "pending", true},
		{"uploaded inválido (OCR anterior)", "uploaded", true},
		{"vacío sin filtro", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repoCalled := false
			uc := NewUseCase(UseCaseDeps{
				DocRepo: &testDocRepo{
					getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
						if tc.status == "" {
							repoCalled = true
						}
						return []*domain.UserDocument{}, nil
					},
					getByUserIDFilteredFn: func(ctx context.Context, uid uuid.UUID, s domain.OCRStatus, dt string) ([]*domain.UserDocument, error) {
						repoCalled = true
						return []*domain.UserDocument{}, nil
					},
				},
			})
			_, err := uc.Execute(t.Context(), userID.String(), tc.status, "")
			if tc.expectErr {
				if err == nil {
					t.Errorf("status=%q debería haber fallado", tc.status)
				}
				return
			}
			if err != nil {
				t.Errorf("status=%q no debería fallar: %v", tc.status, err)
			}
			if !repoCalled {
				t.Error("repo debería haber sido llamado con filtro válido")
			}
		})
	}
}

func TestListDocumentsUseCase_DocTypeFilterValidation(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name      string
		docType   string
		expectErr bool
	}{
		{"passport válido", "passport", false},
		{"national_id válido", "national_id", false},
		{"drivers_license válido", "drivers_license", false},
		{"visa válido", "visa", false},
		{"travel_insurance válido", "travel_insurance", false},
		{"vaccination_cert válido", "vaccination_cert", false},
		{"boarding_pass válido", "boarding_pass", false},
		{"receipt válido", "receipt", false},
		{"invalid inválido", "invalid_type", true},
		{"pasaporte (español) inválido", "pasaporte", true},
		{"vacío sin filtro", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewUseCase(UseCaseDeps{
				DocRepo: &testDocRepo{
					getByUserIDFn: func(ctx context.Context, uid uuid.UUID) ([]*domain.UserDocument, error) {
						return []*domain.UserDocument{}, nil
					},
					getByUserIDFilteredFn: func(ctx context.Context, uid uuid.UUID, s domain.OCRStatus, dt string) ([]*domain.UserDocument, error) {
						return []*domain.UserDocument{}, nil
					},
				},
			})
			_, err := uc.Execute(t.Context(), userID.String(), "", tc.docType)
			if tc.expectErr {
				if err == nil {
					t.Errorf("document_type=%q debería haber fallado", tc.docType)
				}
				return
			}
			if err != nil {
				t.Errorf("document_type=%q no debería fallar: %v", tc.docType, err)
			}
		})
	}
}

func TestListDocumentsUseCase_CombinedFilterValidation(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name      string
		status    string
		docType   string
		expectErr bool
	}{
		{"ambos válidos", "completed", "passport", false},
		{"status inválido + docType válido", "invalid", "passport", true},
		{"status válido + docType inválido", "completed", "invalid", true},
		{"ambos inválidos", "invalid", "invalid", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewUseCase(UseCaseDeps{
				DocRepo: &testDocRepo{
					getByUserIDFilteredFn: func(ctx context.Context, uid uuid.UUID, s domain.OCRStatus, dt string) ([]*domain.UserDocument, error) {
						return []*domain.UserDocument{}, nil
					},
				},
			})
			_, err := uc.Execute(t.Context(), userID.String(), tc.status, tc.docType)
			if tc.expectErr {
				if err == nil {
					t.Errorf("status=%q docType=%q debería haber fallado", tc.status, tc.docType)
				}
				return
			}
			if err != nil {
				t.Errorf("status=%q docType=%q no debería fallar: %v", tc.status, tc.docType, err)
			}
		})
	}
}
