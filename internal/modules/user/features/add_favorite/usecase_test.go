// Tests del usecase add_favorite.
// Table-driven: success, invalid entity_type, repo-error.
package add_favorite

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
	createFn func(ctx context.Context, fav *domain.Favorite) error
}

func (m *mockFavoriteRepo) Create(ctx context.Context, fav *domain.Favorite) error {
	if m.createFn != nil {
		return m.createFn(ctx, fav)
	}
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func TestAddFavorite_Success(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	entityID := uuid.Must(uuid.NewV7())

	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			createFn: func(_ context.Context, fav *domain.Favorite) error { return nil },
		},
	})

	resp, err := uc.Execute(t.Context(), Command{
		UserID:     userID.String(),
		EntityID:   entityID.String(),
		EntityType: "hotel",
		Title:      "Hotel Test",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.FavoriteID == "" {
		t.Error("expected non-empty favorite_id")
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestAddFavorite_InvalidEntityType(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{},
	})

	_, err := uc.Execute(t.Context(), Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		EntityID:   uuid.Must(uuid.NewV7()).String(),
		EntityType: "invalid_type",
		Title:      "Test",
	})
	if !errors.Is(err, domain.ErrInvalidFavoriteEntityType) {
		t.Fatalf("expected ErrInvalidFavoriteEntityType, got %v", err)
	}
}

func TestAddFavorite_RepoError(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			createFn: func(_ context.Context, fav *domain.Favorite) error {
				return errors.New("DB error")
			},
		},
	})

	_, err := uc.Execute(t.Context(), Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		EntityID:   uuid.Must(uuid.NewV7()).String(),
		EntityType: "hotel",
		Title:      "Test",
	})
	if err == nil {
		t.Fatal("expected error on repo failure")
	}
}

func TestAddFavorite_InvalidUserID(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{},
	})

	_, err := uc.Execute(t.Context(), Command{
		UserID:     "not-a-uuid",
		EntityID:   uuid.Must(uuid.NewV7()).String(),
		EntityType: "hotel",
		Title:      "Test",
	})
	if err == nil {
		t.Fatal("expected error for invalid user_id")
	}
}

func TestAddFavorite_InvalidEntityID(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{},
	})

	_, err := uc.Execute(t.Context(), Command{
		UserID:     uuid.Must(uuid.NewV7()).String(),
		EntityID:   "not-a-uuid",
		EntityType: "hotel",
		Title:      "Test",
	})
	if err == nil {
		t.Fatal("expected error for invalid entity_id")
	}
}
