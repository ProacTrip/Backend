// Tests del usecase delete_favorite.
// Table-driven: success, not-found, wrong-owner, repo-error.
package delete_favorite

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

type mockFavoriteRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.Favorite, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *mockFavoriteRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Favorite, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockFavoriteRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func TestDeleteFavorite_Success(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	favID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			getByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Favorite, error) {
				return &domain.Favorite{ID: favID, UserID: userID}, nil
			},
			deleteFn: func(_ context.Context, id uuid.UUID) error { return nil },
		},
	})

	err := uc.Execute(t.Context(), userID.String(), favID.String())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestDeleteFavorite_NotFound(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			getByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Favorite, error) {
				return nil, domain.ErrFavoriteNotFound
			},
		},
	})

	err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String())
	if !errors.Is(err, domain.ErrFavoriteNotFound) {
		t.Fatalf("expected ErrFavoriteNotFound, got %v", err)
	}
}

func TestDeleteFavorite_WrongOwner(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	otherUserID := uuid.Must(uuid.NewV7())
	favID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			getByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Favorite, error) {
				return &domain.Favorite{ID: favID, UserID: otherUserID}, nil
			},
		},
	})

	err := uc.Execute(t.Context(), userID.String(), favID.String())
	if !errors.Is(err, domain.ErrFavoriteNotFound) {
		t.Fatalf("expected ErrFavoriteNotFound for wrong owner, got %v", err)
	}
}

func TestDeleteFavorite_RepoError(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	favID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			getByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Favorite, error) {
				return &domain.Favorite{ID: favID, UserID: userID}, nil
			},
			deleteFn: func(_ context.Context, id uuid.UUID) error {
				return errors.New("DB error")
			},
		},
	})

	err := uc.Execute(t.Context(), userID.String(), favID.String())
	if err == nil {
		t.Fatal("expected error on repo failure")
	}
}

func TestDeleteFavorite_InvalidUserID(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{},
	})

	err := uc.Execute(t.Context(), "not-a-uuid", uuid.Must(uuid.NewV7()).String())
	if err == nil {
		t.Fatal("expected error for invalid user_id")
	}
}

func TestDeleteFavorite_InvalidFavoriteID(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{},
	})

	err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()).String(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid favorite_id")
	}
}
