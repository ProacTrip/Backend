// Tests del usecase upsert_profile.
// Cubre creación de perfil, actualización con EnvPrefs, defaults de entidades,
// cache de prefs, y manejo de errores de repositorio.
package upsert_profile

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	sharedUser "github.com/ProacTrip/Backend/internal/shared/user"
)

// =============================================================================
// Mocks
// =============================================================================

type mockProfileRepo struct {
	upsertProfileFn func(ctx context.Context, profile *domain.UserProfile) error
	getByUserIDFn   func(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
}

func (m *mockProfileRepo) Create(_ context.Context, _ *domain.UserProfile) error  { return nil }
func (m *mockProfileRepo) UpsertProfile(ctx context.Context, p *domain.UserProfile) error {
	if m.upsertProfileFn != nil {
		return m.upsertProfileFn(ctx, p)
	}
	return nil
}
func (m *mockProfileRepo) GetByUserID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, id)
	}
	return nil, domain.ErrProfileNotFound
}
func (m *mockProfileRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *mockProfileRepo) Update(_ context.Context, _ *domain.UserProfile) error    { return nil }
func (m *mockProfileRepo) UpdateLocale(_ context.Context, _ uuid.UUID, _, _, _, _ string) error {
	return nil
}
func (m *mockProfileRepo) UpdateAvatar(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (m *mockProfileRepo) UpdatePreferences(_ context.Context, _ uuid.UUID, _, _, _ string, _ bool) error {
	return nil
}

type mockTravelPrefsRepo struct {
	createFn func(ctx context.Context, prefs *domain.TravelPreferences) error
}

func (m *mockTravelPrefsRepo) Create(ctx context.Context, p *domain.TravelPreferences) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	return nil
}
func (m *mockTravelPrefsRepo) GetByUserID(_ context.Context, _ uuid.UUID) (*domain.TravelPreferences, error) {
	return nil, nil
}
func (m *mockTravelPrefsRepo) Update(_ context.Context, _ *domain.TravelPreferences) error { return nil }

type mockMedicalRepo struct {
	createFn func(ctx context.Context, profile *domain.MedicalProfileV2) error
}

func (m *mockMedicalRepo) Create(ctx context.Context, p *domain.MedicalProfileV2) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	return nil
}
func (m *mockMedicalRepo) GetByUserID(_ context.Context, _ uuid.UUID) (*domain.MedicalProfileV2, error) {
	return nil, nil
}
func (m *mockMedicalRepo) Update(_ context.Context, _ *domain.MedicalProfileV2) error { return nil }

type mockNotifPrefsRepo struct {
	upsertFn func(ctx context.Context, pref *domain.NotificationPreference) error
}

func (m *mockNotifPrefsRepo) Create(_ context.Context, _ *domain.NotificationPreference) error { return nil }
func (m *mockNotifPrefsRepo) GetByUserID(_ context.Context, _ uuid.UUID) ([]*domain.NotificationPreference, error) {
	return nil, nil
}
func (m *mockNotifPrefsRepo) Upsert(ctx context.Context, p *domain.NotificationPreference) error {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, p)
	}
	return nil
}
func (m *mockNotifPrefsRepo) Delete(_ context.Context, _ uuid.UUID, _ domain.NotificationChannel, _ domain.NotificationType) error {
	return nil
}

// =============================================================================
// Helper: setup miniredis
// =============================================================================

func setupMiniRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

// =============================================================================
// Tests — Create Profile (Happy Path)
// =============================================================================

func TestUpsertProfile_CreateNew(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	email := "test@example.com"

	var upsertCalled bool
	uc := NewUseCase(&mockProfileRepo{
		upsertProfileFn: func(_ context.Context, p *domain.UserProfile) error {
			upsertCalled = true
			if p.UserID != userID {
				t.Errorf("UserID = %s, want %s", p.UserID, userID)
			}
			if p.Email != email {
				t.Errorf("Email = %q, want %q", p.Email, email)
			}
			if p.CurrencyCode != domain.DefaultCurrency {
				t.Errorf("default CurrencyCode = %q, want %q", p.CurrencyCode, domain.DefaultCurrency)
			}
			return nil
		},
	})

	if err := uc.Execute(t.Context(), userID, email); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !upsertCalled {
		t.Fatal("UpsertProfile not called")
	}
}

func TestUpsertProfile_CreateWithEnvPrefs(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	email := "test@example.com"

	uc := NewUseCase(&mockProfileRepo{
		upsertProfileFn: func(_ context.Context, p *domain.UserProfile) error {
			if p.TimezoneName != "Europe/Madrid" {
				t.Errorf("TimezoneName = %q, want %q", p.TimezoneName, "Europe/Madrid")
			}
			if p.LanguageCode != "es" {
				t.Errorf("LanguageCode = %q, want %q", p.LanguageCode, "es")
			}
			if p.CurrencyCode != "EUR" {
				t.Errorf("CurrencyCode = %q, want %q", p.CurrencyCode, "EUR")
			}
			return nil
		},
	})

	envPrefs := domain.EnvPrefs{
		LanguageCode: "es",
		CurrencyCode: "EUR",
		TimezoneName: "Europe/Madrid",
	}

	if err := uc.Execute(t.Context(), userID, email, envPrefs); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
}

func TestUpsertProfile_RepoError(t *testing.T) {
	uc := NewUseCase(&mockProfileRepo{
		upsertProfileFn: func(_ context.Context, _ *domain.UserProfile) error {
			return errors.New("DB connection lost")
		},
	})

	err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()), "")
	if err == nil {
		t.Fatal("expected error on repo failure")
	}
}

// =============================================================================
// Tests — Cache Interaction
// =============================================================================

func TestUpsertProfile_PopulatesCache(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	rdb, _ := setupMiniRedis(t)

	uc := NewUseCaseWithCache(&mockProfileRepo{}, rdb)
	if err := uc.Execute(t.Context(), userID, ""); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	// Verify cache was populated
	prefs, err := sharedUser.GetProfilePrefs(t.Context(), rdb, userID.String())
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if prefs == nil {
		t.Fatal("cache not populated after upsert")
	}
	if prefs.Currency != domain.DefaultCurrency {
		t.Errorf("cached Currency = %q, want %q", prefs.Currency, domain.DefaultCurrency)
	}
	if prefs.Language != domain.DefaultLanguage {
		t.Errorf("cached Language = %q, want %q", prefs.Language, domain.DefaultLanguage)
	}
}

func TestUpsertProfile_NoCacheWithoutRDB(t *testing.T) {
	// Use constructor that doesn't set rdb
	uc := NewUseCase(&mockProfileRepo{})
	if err := uc.Execute(t.Context(), uuid.Must(uuid.NewV7()), ""); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// Should not panic — cache is optional
}

// =============================================================================
// Tests — Complete Profile with All Entities
// =============================================================================

func TestUpsertProfile_CreatesAllDefaults(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	var (
		travelCreated  bool
		medicalCreated bool
		notifUp1Called bool
		notifUp2Called bool
	)

	uc := NewUseCaseComplete(
		&mockProfileRepo{},
		&mockTravelPrefsRepo{
			createFn: func(_ context.Context, p *domain.TravelPreferences) error {
				travelCreated = true
				return nil
			},
		},
		&mockMedicalRepo{
			createFn: func(_ context.Context, p *domain.MedicalProfileV2) error {
				medicalCreated = true
				return nil
			},
		},
		&mockNotifPrefsRepo{
			upsertFn: func(_ context.Context, p *domain.NotificationPreference) error {
				if p.NotificationType == domain.NotifTypeBookingConfirmation {
					notifUp1Called = true
				}
				if p.NotificationType == domain.NotifTypeFlightReminder {
					notifUp2Called = true
				}
				return nil
			},
		},
		nil,
	)

	if err := uc.Execute(t.Context(), userID, ""); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !travelCreated {
		t.Error("travel prefs default not created")
	}
	if !medicalCreated {
		t.Error("medical profile default not created")
	}
	if !notifUp1Called {
		t.Error("notification pref (booking) default not created")
	}
	if !notifUp2Called {
		t.Error("notification pref (flight) default not created")
	}
}

func TestUpsertProfile_DefaultsGracefulOnError(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	// Travel repo fails — should not block profile creation
	uc := NewUseCaseComplete(
		&mockProfileRepo{},
		&mockTravelPrefsRepo{
			createFn: func(_ context.Context, _ *domain.TravelPreferences) error {
				return errors.New("travel repo unavailable")
			},
		},
		&mockMedicalRepo{},
		&mockNotifPrefsRepo{},
		nil,
	)

	// Should succeed — defaults are best-effort
	if err := uc.Execute(t.Context(), userID, ""); err != nil {
		t.Fatalf("Execute should succeed even when defaults fail: %v", err)
	}
}

// =============================================================================
// Tests — HandleVerification
// =============================================================================

func TestHandleVerification_CreatesProfileWhenMissing(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	var upsertCalled bool

	uc := NewUseCase(&mockProfileRepo{
		getByUserIDFn: func(_ context.Context, _ uuid.UUID) (*domain.UserProfile, error) {
			return nil, domain.ErrProfileNotFound
		},
		upsertProfileFn: func(_ context.Context, _ *domain.UserProfile) error {
			upsertCalled = true
			return nil
		},
	})

	if err := uc.HandleVerification(t.Context(), userID); err != nil {
		t.Fatalf("HandleVerification error: %v", err)
	}
	if !upsertCalled {
		t.Fatal("expected UpsertProfile to be called for missing profile")
	}
}

func TestHandleVerification_NoOpWhenProfileExists(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	var upsertCalled bool

	uc := NewUseCase(&mockProfileRepo{
		getByUserIDFn: func(_ context.Context, _ uuid.UUID) (*domain.UserProfile, error) {
			return &domain.UserProfile{}, nil
		},
		upsertProfileFn: func(_ context.Context, _ *domain.UserProfile) error {
			upsertCalled = true
			return nil
		},
	})

	if err := uc.HandleVerification(t.Context(), userID); err != nil {
		t.Fatalf("HandleVerification error: %v", err)
	}
	if upsertCalled {
		t.Fatal("UpsertProfile should NOT be called when profile already exists")
	}
}
