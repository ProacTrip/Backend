// RED — Test del usecase update_locale.
package update_locale

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

type mockProfileRepo struct {
	updateLocaleFn func(ctx context.Context, userID uuid.UUID, tz, lang, cur, loc string) error
}

func (m *mockProfileRepo) Create(ctx context.Context, p *domain.UserProfile) error  { return nil }
func (m *mockProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *mockProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) { return nil, nil }
func (m *mockProfileRepo) Update(ctx context.Context, p *domain.UserProfile) error               { return nil }
func (m *mockProfileRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, tz, lang, cur, loc string) error {
	if m.updateLocaleFn != nil {
		return m.updateLocaleFn(ctx, userID, tz, lang, cur, loc)
	}
	return nil
}
func (m *mockProfileRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func TestUpdateLocale_ValidTimezone(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	tz := "America/Argentina/Buenos_Aires"
	lang := "es"
	cur := "ARS"

	called := false
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			updateLocaleFn: func(ctx context.Context, id uuid.UUID, timezone, language, currency string) error {
				called = true
				if timezone != tz {
					t.Errorf("timezone = %s, esperado %s", timezone, tz)
				}
				if language != lang {
					t.Errorf("language = %s, esperado %s", language, lang)
				}
				if currency != cur {
					t.Errorf("currency = %s, esperado %s", currency, currency)
				}
				return nil
			},
		},
	})

	cmd := Command{
		UserID:       userID.String(),
		TimezoneName: &tz,
		LanguageCode: &lang,
		CurrencyCode: &cur,
	}
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("UpdateLocale debería haber sido llamado")
	}
}

func TestUpdateLocale_InvalidTimezone(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	invalidTZ := "not-a-valid-timezone" // no contiene "/"

	cmd := Command{
		UserID:       userID.String(),
		TimezoneName: &invalidTZ,
	}
	uc := NewUseCase(UseCaseDeps{ProfileRepo: &mockProfileRepo{}})

	err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrInvalidTimezone) {
		t.Errorf("se esperaba ErrInvalidTimezone, obtuve %v", err)
	}
}

func TestUpdateLocale_InvalidLanguageCode(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	invalidLang := "x" // demasiado corto

	cmd := Command{
		UserID:       userID.String(),
		LanguageCode: &invalidLang,
	}
	uc := NewUseCase(UseCaseDeps{ProfileRepo: &mockProfileRepo{}})

	err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrInvalidLanguageCode) {
		t.Errorf("se esperaba ErrInvalidLanguageCode, obtuve %v", err)
	}
}

func TestUpdateLocale_InvalidCurrencyCode(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	invalidCur := "XX" // largo incorrecto (3 chars req)

	cmd := Command{
		UserID:       userID.String(),
		CurrencyCode: &invalidCur,
	}
	uc := NewUseCase(UseCaseDeps{ProfileRepo: &mockProfileRepo{}})

	// XX tiene 2 chars, debe fallar porque se requieren 3
	err := uc.Execute(t.Context(), cmd)
	if !errors.Is(err, domain.ErrInvalidCurrencyCode) {
		t.Errorf("se esperaba ErrInvalidCurrencyCode, obtuve %v", err)
	}
}

func TestUpdateLocale_OnlyPartialFields(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	tz := "Europe/Madrid"

	called := false
	uc := NewUseCase(UseCaseDeps{
		ProfileRepo: &mockProfileRepo{
			updateLocaleFn: func(ctx context.Context, id uuid.UUID, timezone, language, currency string) error {
				called = true
				if timezone != tz {
					t.Error("timezone mal")
				}
				if language != "" {
					t.Error("language debería ser vacío")
				}
				return nil
			},
		},
	})

	cmd := Command{
		UserID:       userID.String(),
		TimezoneName: &tz,
	}
	err := uc.Execute(t.Context(), cmd)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !called {
		t.Error("UpdateLocale debería haber sido llamado")
	}
}
