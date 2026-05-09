// Tests del usecase create_saved_search.
package create_saved_search

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockSavedSearchRepo struct {
	createFn    func(ctx context.Context, search *domain.SavedSearch) error
	getByHashFn func(ctx context.Context, userID uuid.UUID, searchHash string) (*domain.SavedSearch, error)
}

func (m *mockSavedSearchRepo) Create(ctx context.Context, search *domain.SavedSearch) error {
	if m.createFn != nil {
		return m.createFn(ctx, search)
	}
	return nil
}

func (m *mockSavedSearchRepo) GetByHash(ctx context.Context, userID uuid.UUID, searchHash string) (*domain.SavedSearch, error) {
	if m.getByHashFn != nil {
		return m.getByHashFn(ctx, userID, searchHash)
	}
	return nil, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestCreateSavedSearch_HappyPath(t *testing.T) {
	params := json.RawMessage(`{"origin":"EZE","destination":"MAD"}`)
	cmd := Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		Parameters: params,
	}

	called := false
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByHashFn: func(ctx context.Context, userID uuid.UUID, searchHash string) (*domain.SavedSearch, error) {
				return nil, nil
			},
			createFn: func(ctx context.Context, search *domain.SavedSearch) error {
				called = true
				if search.SearchHash == "" {
					t.Error("SearchHash no debería estar vacío")
				}
				if len(search.Parameters) == 0 {
					t.Error("Parameters no debería estar vacío")
				}
				return nil
			},
		},
	})

	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("Create debería haber sido llamado")
	}
	if resp == nil {
		t.Fatal("respuesta no debería ser nil")
	}
	if resp.SearchID == "" {
		t.Error("search_id no debería estar vacío")
	}
}

func TestCreateSavedSearch_Duplicate(t *testing.T) {
	params := json.RawMessage(`{"origin":"EZE","destination":"MAD"}`)
	userID := uuid.Must(uuid.NewV7())
	cmd := Command{
		UserID:     userID.String(),
		Parameters: params,
	}

	// Pre-crear una búsqueda con el mismo hash
	paramsMap := map[string]any{"origin": "EZE", "destination": "MAD"}
	existingHash := domain.GenerateSearchHash(paramsMap)

	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByHashFn: func(ctx context.Context, uid uuid.UUID, hash string) (*domain.SavedSearch, error) {
				if hash == existingHash {
					return &domain.SavedSearch{ID: uuid.Must(uuid.NewV7()), UserID: userID}, nil
				}
				return nil, nil
			},
		},
	})

	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error de duplicado")
	}
	if !errors.Is(err, domain.ErrDuplicateSavedSearch) {
		t.Errorf("error = %v, se esperaba ErrDuplicateSavedSearch", err)
	}
}

func TestCreateSavedSearch_ValidJson(t *testing.T) {
	userID := uuid.Must(uuid.NewV7()).String()

	tests := []struct {
		name       string
		params     json.RawMessage
		filters    json.RawMessage
		searchType *string
		wantErr    bool
		errMsg     string
	}{
		{"parámetros válidos", json.RawMessage(`{"key":"value"}`), nil, nil, false, ""},
		{"parámetros vacíos", json.RawMessage(``), nil, nil, true, "parameters es requerido"},
		{"parámetros nil", json.RawMessage(nil), nil, nil, true, "parameters es requerido"},
		{"JSON inválido", json.RawMessage(`{bad}`), nil, nil, true, "no es JSON válido"},
		{"filters válido", json.RawMessage(`{"key":"value"}`), json.RawMessage(`{"max":100}`), nil, false, ""},
		{"filters inválido", json.RawMessage(`{"key":"value"}`), json.RawMessage(`{bad}`), nil, true, "no es JSON válido"},
		{"search_type flight", json.RawMessage(`{"key":"value"}`), nil, ptr("flight"), false, ""},
		{"search_type hotel", json.RawMessage(`{"key":"value"}`), nil, ptr("hotel"), false, ""},
		{"search_type ai", json.RawMessage(`{"key":"value"}`), nil, ptr("ai"), false, ""},
		{"search_type both", json.RawMessage(`{"key":"value"}`), nil, ptr("both"), false, ""},
		{"search_type inválido", json.RawMessage(`{"key":"value"}`), nil, ptr("invalid"), true, "search_type inválido"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := Command{
				UserID:     userID,
				Parameters: tc.params,
				Filters:    tc.filters,
				SearchType: tc.searchType,
			}
			err := cmd.Validate()
			if tc.wantErr && err == nil {
				t.Error("se esperaba error de validación")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("no se esperaba error: %v", err)
			}
		})
	}
}

func TestCreateSavedSearch_WithName(t *testing.T) {
	params := json.RawMessage(`{"origin":"EZE"}`)
	name := "Mi búsqueda"
	cmd := Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		Name:       &name,
		Parameters: params,
	}

	var createdName string
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByHashFn: func(ctx context.Context, userID uuid.UUID, searchHash string) (*domain.SavedSearch, error) {
				return nil, nil
			},
			createFn: func(ctx context.Context, search *domain.SavedSearch) error {
				if search.Name != nil {
					createdName = *search.Name
				}
				return nil
			},
		},
	})

	_, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if createdName != "Mi búsqueda" {
		t.Errorf("nombre = %q, se esperaba %q", createdName, "Mi búsqueda")
	}
}

func ptr[T any](v T) *T { return &v }
