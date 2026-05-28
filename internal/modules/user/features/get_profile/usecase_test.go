// Test del usecase get_profile.
package get_profile

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Mocks — Implementaciones falsas de los repositorios
// =============================================================================

type mockProfileRepo struct {
	getByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
}

func (m *mockProfileRepo) Create(ctx context.Context, p *domain.UserProfile) error     { return nil }
func (m *mockProfileRepo) UpsertProfile(ctx context.Context, p *domain.UserProfile) error { return nil }
func (m *mockProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) { return nil, nil }
func (m *mockProfileRepo) Update(ctx context.Context, p *domain.UserProfile) error                { return nil }
func (m *mockProfileRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, lang, curr string) error {
	return nil
}
func (m *mockProfileRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, url string) error { return nil }
func (m *mockProfileRepo) UpdatePreferences(ctx context.Context, userID uuid.UUID, lang, curr string) error {
	return nil
}

// =============================================================================
// Tests — Table-driven
// =============================================================================

func TestGetProfile_HappyPath(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	profile := domain.NewUserProfile(userID, "test@example.com")
	profile.FirstName = new("María")
	profile.LastName = new("Gómez")
	profile.LanguageCode = "es"
	profile.CurrencyCode = "ARS"

	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return profile, nil
			},
		},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp == nil {
		t.Fatal("response no debería ser nil")
	}
	if resp.FirstName == nil || *resp.FirstName != "María" {
		t.Errorf("FirstName = %v, se esperaba María", resp.FirstName)
	}
	if resp.Location.Currency != "ARS" {
		t.Errorf("Location.Currency = %s, se esperaba ARS", resp.Location.Currency)
	}
	if resp.Location.Language != "es" {
		t.Errorf("Location.Language = %s, se esperaba es", resp.Location.Language)
	}
	if resp.RoleName != "client" {
		t.Errorf("RoleName = %s, se esperaba client", resp.RoleName)
	}
}

func TestGetProfile_AdminRole(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	profile := domain.NewUserProfile(userID, "admin@example.com")
	profile.Role = "admin"
	profile.FirstName = new("Admin")
	profile.LastName = new("User")
	profile.LanguageCode = "en"
	profile.CurrencyCode = "USD"

	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return profile, nil
			},
		},
	})

	cmd := Command{UserID: userID.String()}
	resp, err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp == nil {
		t.Fatal("response no debería ser nil")
	}
	if resp.RoleName != "admin" {
		t.Errorf("RoleName = %s, se esperaba admin", resp.RoleName)
	}
}

func TestGetProfile_ProfileNotFound(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return nil, domain.ErrProfileNotFound
			},
		},
	})

	cmd := Command{UserID: uuid.Must(uuid.NewV7()).String()}
	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error ErrProfileNotFound")
	}
	if !errors.Is(err, domain.ErrProfileNotFound) {
		t.Errorf("error = %v, se esperaba ErrProfileNotFound", err)
	}
}

func TestGetProfile_NilProfileReturnsError(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			getByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
				return nil, nil
			},
		},
	})

	cmd := Command{UserID: uuid.Must(uuid.NewV7()).String()}
	_, err := uc.Execute(t.Context(), cmd)
	if err == nil {
		t.Fatal("se esperaba error para perfil nil")
	}
}
