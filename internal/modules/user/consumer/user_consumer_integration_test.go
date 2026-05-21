// Integration test: full registration → profile prefs pipeline.
package consumer_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/consumer"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upsert_profile"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	sharedUser "github.com/ProacTrip/Backend/internal/shared/user"
)

// =============================================================================
// Mocks para la integración consumer→profile→cache
// =============================================================================

type integrationMockRepo struct {
	profiles map[uuid.UUID]*domain.UserProfile
}

func newIntegrationMockRepo() *integrationMockRepo {
	return &integrationMockRepo{
		profiles: make(map[uuid.UUID]*domain.UserProfile),
	}
}

func (m *integrationMockRepo) UpsertProfile(ctx context.Context, profile *domain.UserProfile) error {
	if profile == nil {
		return fmt.Errorf("nil profile")
	}
	m.profiles[profile.UserID] = profile
	return nil
}

func (m *integrationMockRepo) Create(ctx context.Context, profile *domain.UserProfile) error {
	return m.UpsertProfile(ctx, profile)
}

func (m *integrationMockRepo) Update(ctx context.Context, profile *domain.UserProfile) error {
	if profile == nil {
		return fmt.Errorf("nil profile")
	}
	p, ok := m.profiles[profile.UserID]
	if !ok {
		return fmt.Errorf("profile not found")
	}
	if profile.FirstName != nil {
		p.FirstName = profile.FirstName
	}
	if profile.LastName != nil {
		p.LastName = profile.LastName
	}
	if profile.DateOfBirth != nil {
		p.DateOfBirth = profile.DateOfBirth
	}
	if profile.Gender != nil {
		p.Gender = profile.Gender
	}
	if profile.Nationality != nil {
		p.Nationality = profile.Nationality
	}
	if profile.Phone != nil {
		p.Phone = profile.Phone
	}
	if profile.Bio != nil {
		p.Bio = profile.Bio
	}
	return nil
}

func (m *integrationMockRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, language, currency string) error {
	p, ok := m.profiles[userID]
	if !ok {
		return fmt.Errorf("profile not found")
	}
	if language != "" {
		p.LanguageCode = language
	}
	if currency != "" {
		p.CurrencyCode = currency
	}
	return nil
}

func (m *integrationMockRepo) UpdatePreferences(ctx context.Context, userID uuid.UUID, language, currency string) error {
	return m.UpdateLocale(ctx, userID, language, currency)
}

func (m *integrationMockRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	p, ok := m.profiles[userID]
	if !ok {
		return nil, fmt.Errorf("profile not found")
	}
	return p, nil
}

func (m *integrationMockRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
	for _, p := range m.profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("profile not found")
}

func (m *integrationMockRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	return nil
}

// =============================================================================
// Task 6.1 — Integration: registration event → profile + cache
// =============================================================================

func TestIntegration_RegistrationEventToProfileCache(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	envPrefs := domain.EnvPrefs{
		CurrencyCode: "EUR",
		LanguageCode: "es",
		CountryCode:  "ES",
		TimezoneName: "Europe/Madrid",
	}

	if err := uc.Execute(t.Context(), userID, "test@example.com", "", "", envPrefs); err != nil {
		t.Fatalf("upsert profile failed: %v", err)
	}

	profile, err := repo.GetByUserID(t.Context(), userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if profile.CurrencyCode != "EUR" {
		t.Errorf("profile.CurrencyCode = %q, want %q", profile.CurrencyCode, "EUR")
	}
	if profile.LanguageCode != "es" {
		t.Errorf("profile.LanguageCode = %q, want %q", profile.LanguageCode, "es")
	}
	if profile.UserID != userID {
		t.Errorf("profile.UserID = %s, want %s", profile.UserID, userID)
	}

	// Verify Dragonfly cache
	prefs, err := sharedUser.GetProfilePrefs(t.Context(), rdb, userID.String())
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if prefs == nil {
		t.Fatal("profile prefs cache NOT populated")
	}
	if prefs.Currency != "EUR" {
		t.Errorf("cache currency = %q, want %q", prefs.Currency, "EUR")
	}
	if prefs.Language != "es" {
		t.Errorf("cache language = %q, want %q", prefs.Language, "es")
	}
}

func TestIntegration_RegistrationEventWithoutEnvFields_ProfileDefaults(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	if err := uc.Execute(t.Context(), userID, "", "", ""); err != nil {
		t.Fatalf("upsert profile failed: %v", err)
	}

	profile, err := repo.GetByUserID(t.Context(), userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if profile.CurrencyCode != domain.DefaultCurrency {
		t.Errorf("CurrencyCode = %q, want %q", profile.CurrencyCode, domain.DefaultCurrency)
	}
	if profile.LanguageCode != domain.DefaultLanguage {
		t.Errorf("LanguageCode = %q, want %q", profile.LanguageCode, domain.DefaultLanguage)
	}

	prefs, _ := sharedUser.GetProfilePrefs(t.Context(), rdb, userID.String())
	if prefs == nil {
		t.Fatal("cache should exist even for legacy events")
	}
}

func TestIntegration_RegistrationEventConsumerExtraction(t *testing.T) {
	payload := map[string]interface{}{
		"event_type":         "user_registered",
		"aggregate_id":       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"timestamp":          float64(1715000000000),
		"user_id":            "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"email":              "brazilian@example.com",
		"verification_token": "vt-123",
		"language_code":      "pt",
		"currency_code":      "BRL",
		"country_code":       "BR",
		"timezone_name":      "America/Sao_Paulo",
	}

	prefs := consumer.ExtractEnvPrefsForTest(payload)

	if !prefs.HasAny() {
		t.Fatal("expected HasAny=true for full env payload")
	}
	if prefs.LanguageCode != "pt" {
		t.Errorf("LanguageCode = %q, want %q", prefs.LanguageCode, "pt")
	}
	if prefs.CurrencyCode != "BRL" {
		t.Errorf("CurrencyCode = %q, want %q", prefs.CurrencyCode, "BRL")
	}
	if prefs.CountryCode != "BR" {
		t.Errorf("CountryCode = %q, want %q", prefs.CountryCode, "BR")
	}
	if prefs.TimezoneName != "America/Sao_Paulo" {
		t.Errorf("TimezoneName = %q, want %q", prefs.TimezoneName, "America/Sao_Paulo")
	}
}

func TestIntegration_RegistrationEventMixedStream(t *testing.T) {
	userIDOld := uuid.Must(uuid.NewV7())
	userIDNew := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	// Process old-style event (no env fields)
	if err := uc.Execute(t.Context(), userIDOld, "", "", ""); err != nil {
		t.Fatalf("old event upsert failed: %v", err)
	}

	// Process new-style event (with env fields)
	newEnvPrefs := domain.EnvPrefs{
		CurrencyCode: "MXN",
		LanguageCode: "es",
		CountryCode:  "MX",
		TimezoneName: "America/Mexico_City",
	}
	if err := uc.Execute(t.Context(), userIDNew, "", "", "", newEnvPrefs); err != nil {
		t.Fatalf("new event upsert failed: %v", err)
	}

	oldProfile, _ := repo.GetByUserID(t.Context(), userIDOld)
	if oldProfile.CurrencyCode != domain.DefaultCurrency {
		t.Errorf("old profile currency = %q, want %q", oldProfile.CurrencyCode, domain.DefaultCurrency)
	}
	if oldProfile.LanguageCode != domain.DefaultLanguage {
		t.Errorf("old profile language = %q, want %q", oldProfile.LanguageCode, domain.DefaultLanguage)
	}

	newProfile, _ := repo.GetByUserID(t.Context(), userIDNew)
	if newProfile.CurrencyCode != "MXN" {
		t.Errorf("new profile currency = %q, want %q", newProfile.CurrencyCode, "MXN")
	}
	if newProfile.LanguageCode != "es" {
		t.Errorf("new profile language = %q, want %q", newProfile.LanguageCode, "es")
	}

	// Both caches should be populated
	oldPrefs, _ := sharedUser.GetProfilePrefs(t.Context(), rdb, userIDOld.String())
	if oldPrefs == nil {
		t.Fatal("old event: cache not populated")
	}

	newPrefs, _ := sharedUser.GetProfilePrefs(t.Context(), rdb, userIDNew.String())
	if newPrefs == nil {
		t.Fatal("new event: cache not populated")
	}
	if newPrefs.Currency != "MXN" {
		t.Errorf("new event: cached currency = %q, want %q", newPrefs.Currency, "MXN")
	}
}

func TestIntegration_LegacyEventReplayDoesNotCrash(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	if err := uc.Execute(t.Context(), userID, "", "", ""); err != nil {
		t.Fatalf("legacy event replay should not crash: %v", err)
	}

	profile, err := repo.GetByUserID(t.Context(), userID)
	if err != nil {
		t.Fatalf("profile should exist after legacy replay: %v", err)
	}
	validDefault := domain.NewUserProfile(userID, "")
	if profile.CurrencyCode != validDefault.CurrencyCode {
		t.Errorf("CurrencyCode = %q, want hardcoded %q", profile.CurrencyCode, validDefault.CurrencyCode)
	}
	if profile.LanguageCode != validDefault.LanguageCode {
		t.Errorf("LanguageCode = %q, want hardcoded %q", profile.LanguageCode, validDefault.LanguageCode)
	}
}

func TestIntegration_EventStructureBackwardCompatible(t *testing.T) {
	// Event without env fields — old format
	legacyEvent := eventbus.NewUserRegisteredEvent(
		"user-legacy",
		"old@example.com",
		"token-123",
		"", "", "", "", // all env fields empty
	)

	if v, ok := legacyEvent.Payload["user_id"]; !ok || v != "user-legacy" {
		t.Errorf("legacy payload user_id = %v", v)
	}

	for _, key := range []string{"language_code", "currency_code", "country_code", "timezone_name"} {
		if _, ok := legacyEvent.Payload[key]; ok {
			t.Errorf("legacy event should NOT contain %q when empty", key)
		}
	}

	// Event with env fields — new format
	newEvent := eventbus.NewUserRegisteredEvent(
		"user-new",
		"new@example.com",
		"token-456",
		"es", "EUR", "ES", "Europe/Madrid",
	)

	if v := newEvent.Payload["language_code"]; v != "es" {
		t.Errorf("new payload language_code = %v, want 'es'", v)
	}
	if v := newEvent.Payload["currency_code"]; v != "EUR" {
		t.Errorf("new payload currency_code = %v, want 'EUR'", v)
	}
}
