// Tests del usecase delete_saved_search.
// Table-driven: success, not-found, wrong-owner, repo-error.
package delete_saved_search

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks
// =============================================================================

type mockSavedSearchRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *mockSavedSearchRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockSavedSearchRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func TestDeleteSavedSearch_Success(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	searchID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByIDFn: func(_ context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
				return &domain.SavedSearch{ID: searchID, UserID: userID}, nil
			},
			deleteFn: func(_ context.Context, id uuid.UUID) error { return nil },
		},
	})

	err := uc.Execute(t.Context(), userID.String(), searchID.String())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestDeleteSavedSearch_NotFound(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByIDFn: func(_ context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
				return nil, domain.ErrSearchNotFound
			},
		},
	})

	err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String())
	if !errors.Is(err, domain.ErrSearchNotFound) {
		t.Fatalf("expected ErrSearchNotFound, got %v", err)
	}
}

func TestDeleteSavedSearch_WrongOwner(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	otherUserID := uuid.Must(uuid.NewV7())
	searchID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByIDFn: func(_ context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
				return &domain.SavedSearch{ID: searchID, UserID: otherUserID}, nil
			},
		},
	})

	err := uc.Execute(t.Context(), userID.String(), searchID.String())
	if !errors.Is(err, domain.ErrSearchNotFound) {
		t.Fatalf("expected ErrSearchNotFound for wrong owner, got %v", err)
	}
}

func TestDeleteSavedSearch_RepoError(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	searchID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{
			getByIDFn: func(_ context.Context, id uuid.UUID) (*domain.SavedSearch, error) {
				return &domain.SavedSearch{ID: searchID, UserID: userID}, nil
			},
			deleteFn: func(_ context.Context, id uuid.UUID) error {
				return errors.New("DB error")
			},
		},
	})

	err := uc.Execute(t.Context(), userID.String(), searchID.String())
	if err == nil {
		t.Fatal("expected error on repo failure")
	}
}

func TestDeleteSavedSearch_InvalidUserID(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{},
	})

	err := uc.Execute(t.Context(), "not-a-uuid", uuid.Must(uuid.NewV7()).String())
	if err == nil {
		t.Fatal("expected error for invalid user_id")
	}
}

func TestDeleteSavedSearch_InvalidSearchID(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		SavedSearchRepo: &mockSavedSearchRepo{},
	})

	err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()).String(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid search_id")
	}
}
