// Tests del usecase list_favorites.
// Table-driven: success, empty, filtered, repo-error.
package list_favorites

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

type mockFavoriteRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) ([]*domain.Favorite, error)
}

func (m *mockFavoriteRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Favorite, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}

// =============================================================================
// Helpers
// =============================================================================

func makeFavorite() *domain.Favorite {
	now := time.Now()
	return &domain.Favorite{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     uuid.Must(uuid.NewV7()),
		EntityID:   uuid.Must(uuid.NewV7()),
		EntityType: "hotel",
		Title:      "Hotel Test",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestListFavorites_Success(t *testing.T) {
	fav := makeFavorite()
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			getByUserIDFn: func(_ context.Context, userID uuid.UUID) ([]*domain.Favorite, error) {
				return []*domain.Favorite{fav}, nil
			},
		},
	})

	resp, err := uc.Execute(t.Context(), fav.UserID.String(), nil)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(resp.Favorites) != 1 {
		t.Fatalf("expected 1 favorite, got %d", len(resp.Favorites))
	}
	if resp.Favorites[0].Title != "Hotel Test" {
		t.Errorf("title = %q, want %q", resp.Favorites[0].Title, "Hotel Test")
	}
}

func TestListFavorites_Empty(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			getByUserIDFn: func(_ context.Context, userID uuid.UUID) ([]*domain.Favorite, error) {
				return []*domain.Favorite{}, nil
			},
		},
	})

	resp, err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()).String(), nil)
	if err != nil {
		t.Fatalf("expected success for empty list, got %v", err)
	}
	if len(resp.Favorites) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp.Favorites))
	}
}

func TestListFavorites_Filtered(t *testing.T) {
	favHotel := makeFavorite()
	favFlight := makeFavorite()
	favFlight.EntityType = "flight"
	favFlight.Title = "Vuelo Test"

	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			getByUserIDFn: func(_ context.Context, userID uuid.UUID) ([]*domain.Favorite, error) {
				return []*domain.Favorite{favHotel, favFlight}, nil
			},
		},
	})

	filter := "flight"
	resp, err := uc.Execute(t.Context(), favHotel.UserID.String(), &filter)
	if err != nil {
		t.Fatalf("expected success with filter, got %v", err)
	}
	if len(resp.Favorites) != 1 {
		t.Fatalf("expected 1 filtered favorite, got %d", len(resp.Favorites))
	}
	if resp.Favorites[0].EntityType != "flight" {
		t.Errorf("entity_type = %q, want %q", resp.Favorites[0].EntityType, "flight")
	}
}

func TestListFavorites_RepoError(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{
			getByUserIDFn: func(_ context.Context, userID uuid.UUID) ([]*domain.Favorite, error) {
				return nil, errors.New("DB error")
			},
		},
	})

	_, err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()).String(), nil)
	if err == nil {
		t.Fatal("expected error on repo failure")
	}
}

func TestListFavorites_InvalidUserID(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		FavoriteRepo: &mockFavoriteRepo{},
	})

	_, err := uc.Execute(t.Context(), "not-a-uuid", nil)
	if err == nil {
		t.Fatal("expected error for invalid user_id")
	}
}
