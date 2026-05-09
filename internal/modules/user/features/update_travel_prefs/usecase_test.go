// RED — Test del usecase update_travel_prefs.
package update_travel_prefs

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

type mockTravelPrefsRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error)
	createFn      func(ctx context.Context, p *domain.TravelPreferences) error
	updateFn      func(ctx context.Context, p *domain.TravelPreferences) error
}

func (m *mockTravelPrefsRepo) Create(ctx context.Context, p *domain.TravelPreferences) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	return nil
}
func (m *mockTravelPrefsRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.TravelPreferences, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, domain.ErrProfileNotFound
}
func (m *mockTravelPrefsRepo) Update(ctx context.Context, p *domain.TravelPreferences) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, p)
	}
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func TestUpdateTravelPrefs_HappyPath(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	class := "business"
	seat := "window"

	updated := false
	uc := NewUseCase(UseCaseDeps{
		TravelPrefsRepo: &mockTravelPrefsRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TravelPreferences, error) {
				tp := domain.NewTravelPreferences(userID)
				return tp, nil
			},
			updateFn: func(ctx context.Context, p *domain.TravelPreferences) error {
				updated = true
				if string(p.PreferredClass) != "business" {
					t.Errorf("PreferredClass = %s, esperado business", p.PreferredClass)
				}
				return nil
			},
		},
	})

	cmd := Command{
		UserID:      userID.String(),
		PreferredClass: &class,
		SeatPreference: &seat,
	}
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !updated {
		t.Error("Update debería haber sido llamado")
	}
}

func TestUpdateTravelPrefs_CreatesIfNotExists(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	class := "economy"

	created := false
	uc := NewUseCase(UseCaseDeps{
		TravelPrefsRepo: &mockTravelPrefsRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TravelPreferences, error) {
				return nil, domain.ErrProfileNotFound
			},
			createFn: func(ctx context.Context, p *domain.TravelPreferences) error {
				created = true
				return nil
			},
		},
	})

	cmd := Command{
		UserID:      userID.String(),
		PreferredClass: &class,
	}
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !created {
		t.Error("Create debería haber sido llamado cuando no existen prefs")
	}
}

func TestUpdateTravelPrefs_InvalidClass(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	invalidClass := "supersonic"

	uc := NewUseCase(UseCaseDeps{
		TravelPrefsRepo: &mockTravelPrefsRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TravelPreferences, error) {
				return domain.NewTravelPreferences(userID), nil
			},
		},
	})

	cmd := Command{
		UserID:      userID.String(),
		PreferredClass: &invalidClass,
	}
	err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrInvalidPreferredClass) {
		t.Errorf("se esperaba ErrInvalidPreferredClass, obtuve %v", err)
	}
}

func TestUpdateTravelPrefs_InvalidSeat(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	invalidSeat := "rooftop"

	uc := NewUseCase(UseCaseDeps{
		TravelPrefsRepo: &mockTravelPrefsRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TravelPreferences, error) {
				return domain.NewTravelPreferences(userID), nil
			},
		},
	})

	cmd := Command{
		UserID:      userID.String(),
		SeatPreference: &invalidSeat,
	}
	err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrInvalidSeatPreference) {
		t.Errorf("se esperaba ErrInvalidSeatPreference, obtuve %v", err)
	}
}

func TestUpdateTravelPrefs_ValidEnumValues(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name  string
		class string
		seat  string
		valid bool
	}{
		{"economy + window", "economy", "window", true},
		{"premium_economy + aisle", "premium_economy", "aisle", true},
		{"business + middle", "business", "middle", true},
		{"first + no_preference", "first", "no_preference", true},
		{"invalid class", "luxury", "window", false},
		{"invalid seat", "economy", "door", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := Command{
				UserID:      userID.String(),
				PreferredClass: &tc.class,
				SeatPreference: &tc.seat,
			}
			uc := NewUseCase(UseCaseDeps{
				TravelPrefsRepo: &mockTravelPrefsRepo{
					getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TravelPreferences, error) {
						return domain.NewTravelPreferences(userID), nil
					},
				},
			})
			err := uc.Execute(t.Context(), cmd)
			if tc.valid && err != nil {
				t.Errorf("no se esperaba error: %v", err)
			}
			if !tc.valid && err == nil {
				t.Error("se esperaba error de validación")
			}
		})
	}
}
