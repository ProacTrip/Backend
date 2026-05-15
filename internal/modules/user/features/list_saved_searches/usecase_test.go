// Tests del usecase list_saved_searches.
// Table-driven: success, empty, repo-error.
package list_saved_searches

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockSavedSearchRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error)
}

func (m *mockSavedSearchRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}

// =============================================================================
// Helpers
// =============================================================================

func makeSavedSearch() *domain.SavedSearch {
	now := time.Now()
	name := "Búsqueda Test"
	return &domain.SavedSearch{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    uuid.Must(uuid.NewV7()),
		Name:      &name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestListSavedSearches_Success(t *testing.T) {
	search := makeSavedSearch()
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByUserIDFn: func(_ context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error) {
				return []*domain.SavedSearch{search}, nil
			},
		},
	})

	resp, err := uc.Execute(t.Context(), search.UserID.String())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(resp.Searches) != 1 {
		t.Fatalf("expected 1 saved search, got %d", len(resp.Searches))
	}
}

func TestListSavedSearches_Empty(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByUserIDFn: func(_ context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error) {
				return []*domain.SavedSearch{}, nil
			},
		},
	})

	resp, err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()).String())
	if err != nil {
		t.Fatalf("expected success for empty list, got %v", err)
	}
	if len(resp.Searches) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp.Searches))
	}
}

func TestListSavedSearches_RepoError(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByUserIDFn: func(_ context.Context, userID uuid.UUID) ([]*domain.SavedSearch, error) {
				return nil, errors.New("DB error")
			},
		},
	})

	_, err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()).String())
	if err == nil {
		t.Fatal("expected error on repo failure")
	}
}

func TestListSavedSearches_InvalidUserID(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{},
	})

	_, err := uc.Execute(t.Context(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid user_id")
	}
}
