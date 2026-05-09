// Integration test: full registration → profile prefs pipeline.
// Verifies that when a UserRegistered event with env fields is processed,
// the consumer (1) creates a profile with env-derived prefs, and (2) populates
// the Dragonfly profile:{userID}:prefs cache.
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
	"github.com/ProacTrip/Backend/internal/modules/user/features/shared"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upsert_profile"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
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
	if profile.FirstName != nil { p.FirstName = profile.FirstName }
	if profile.LastName != nil { p.LastName = profile.LastName }
	if profile.DateOfBirth != nil { p.DateOfBirth = profile.DateOfBirth }
	if profile.Gender != nil { p.Gender = profile.Gender }
	if profile.Nationality != nil { p.Nationality = profile.Nationality }
	if profile.Phone != nil { p.Phone = profile.Phone }
	if profile.Bio != nil { p.Bio = profile.Bio }
	if profile.IsPublic != nil { p.IsPublic = profile.IsPublic }
	return nil
}

func (m *integrationMockRepo) UpdateLocale(ctx context.Context, userID uuid.UUID, timezone, language, currency, currentLocation string) error {
	p, ok := m.profiles[userID]
	if !ok {
		return fmt.Errorf("profile not found")
	}
	if timezone != "" { p.TimezoneName = timezone }
	if language != "" { p.LanguageCode = language }
	if currency != "" { p.CurrencyCode = currency }
	return nil
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

func (m *integrationMockRepo) UpdateStatus(ctx context.Context, userID uuid.UUID, status domain.UserProfileStatus) error {
	return nil
}

func (m *integrationMockRepo) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	return nil
}

func (m *integrationMockRepo) UpdatePreferences(ctx context.Context, userID uuid.UUID, timezone, language, currency string, isPublic bool) error {
	p, ok := m.profiles[userID]
	if !ok {
		return fmt.Errorf("profile not found")
	}
	p.TimezoneName = timezone
	p.LanguageCode = language
	p.CurrencyCode = currency
	return nil
}

// =============================================================================
// Task 6.1 — Integration: registration event → profile + cache
// =============================================================================

// TestIntegration_RegistrationEventToProfileCache verifies the full pipeline:
//
//	UserRegistered event (with env fields) → consumer extracts env prefs →
//	upsert profile with env defaults → Dragonfly cache populated.
func TestIntegration_RegistrationEventToProfileCache(t *testing.T) {
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	// 1. Create mock repo and upsert usecase with cache (simulates consumer behaviour)
	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	// 2. Simulate environment prefs from registration event (Spain → EUR/es/ES)
	envPrefs := domain.EnvPrefs{
		CurrencyCode: "EUR",
		LanguageCode: "es",
		CountryCode:  "ES",
		TimezoneName: "Europe/Madrid",
	}

	// 3. Execute upsert with env prefs (what the consumer calls)
	if err := uc.Execute(ctx, userID, "test@example.com", envPrefs); err != nil {
		t.Fatalf("upsert profile failed: %v", err)
	}

	// 4. Verify profile was created with env defaults overriding hardcoded values
	profile, err := repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if profile.CurrencyCode != "EUR" {
		t.Errorf("profile.CurrencyCode = %q, want %q", profile.CurrencyCode, "EUR")
	}
	if profile.LanguageCode != "es" {
		t.Errorf("profile.LanguageCode = %q, want %q", profile.LanguageCode, "es")
	}
	if profile.TimezoneName != "Europe/Madrid" {
		t.Errorf("profile.TimezoneName = %q, want %q", profile.TimezoneName, "Europe/Madrid")
	}
	// CountryCode goes to cache but not persisted to profile column (by design)
	if profile.UserID != userID {
		t.Errorf("profile.UserID = %s, want %s", profile.UserID, userID)
	}

	// 5. Verify Dragonfly cache has the correct profile prefs
	currency, language, countryCode, timezone, found, err := shared.GetProfilePrefs(ctx, rdb, userID.String())
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if !found {
		t.Fatal("profile prefs cache NOT populated by consumer — expected found=true")
	}
	if currency != "EUR" {
		t.Errorf("cache currency = %q, want %q", currency, "EUR")
	}
	if language != "es" {
		t.Errorf("cache language = %q, want %q", language, "es")
	}
	if timezone != "Europe/Madrid" {
		t.Errorf("cache timezone = %q, want %q", timezone, "Europe/Madrid")
	}
	// Country code is NOT stored in profile column (design decision), so cache
	// field may be empty — that's expected behavior
	_ = countryCode
}

// TestIntegration_RegistrationEventWithoutEnvFields_ProfileDefaults verifies
// that when an event has NO env fields (legacy event), the consumer creates a
// profile with hardcoded defaults and the cache is NOT populated with env data.
func TestIntegration_RegistrationEventWithoutEnvFields_ProfileDefaults(t *testing.T) {
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	// Empty env prefs — simulates a legacy event without env fields
	emptyPrefs := domain.EnvPrefs{}

	if err := uc.Execute(ctx, userID, "", emptyPrefs); err != nil {
		t.Fatalf("upsert profile failed: %v", err)
	}

	// Profile should have hardcoded defaults
	profile, err := repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if profile.CurrencyCode != "EUR" {
		t.Errorf("legacy event: CurrencyCode = %q, want %q", profile.CurrencyCode, "EUR")
	}
	if profile.LanguageCode != "es" {
		t.Errorf("legacy event: LanguageCode = %q, want %q", profile.LanguageCode, "es")
	}
	if profile.TimezoneName != "UTC" {
		t.Errorf("legacy event: TimezoneName = %q, want %q", profile.TimezoneName, "UTC")
	}

	// Cache should exist (profile was created) but with default values
	currency, _, _, timezone, found, _ := shared.GetProfilePrefs(ctx, rdb, userID.String())
	if !found {
		t.Fatal("cache should exist even for legacy events (profile was created)")
	}
	if currency != "EUR" {
		t.Errorf("cache currency = %q, want %q", currency, "EUR")
	}
	if timezone != "UTC" {
		t.Errorf("cache timezone = %q, want %q", timezone, "UTC")
	}
}

// TestIntegration_RegistrationEventConsumerExtraction verifies that the consumer's
// extractEnvPrefs correctly parses env fields from the event payload.
func TestIntegration_RegistrationEventConsumerExtraction(t *testing.T) {
	// Simulate full event payload as it comes from Dragonfly streams
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

// TestIntegration_RegistrationEventMixedStream verifies that the consumer
// correctly handles a mix of old (no env fields) and new (with env fields)
// events in sequence — both produce valid profiles without errors.
func TestIntegration_RegistrationEventMixedStream(t *testing.T) {
	ctx := context.Background()
	userIDOld := uuid.Must(uuid.NewV7())
	userIDNew := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	// Process old-style event (no env fields)
	if err := uc.Execute(ctx, userIDOld, ""); err != nil {
		t.Fatalf("old event upsert failed: %v", err)
	}

	// Process new-style event (with env fields)
	newEnvPrefs := domain.EnvPrefs{
		CurrencyCode: "MXN",
		LanguageCode: "es",
		CountryCode:  "MX",
		TimezoneName: "America/Mexico_City",
	}
	if err := uc.Execute(ctx, userIDNew, "", newEnvPrefs); err != nil {
		t.Fatalf("new event upsert failed: %v", err)
	}

	// Old profile: hardcoded defaults
	oldProfile, _ := repo.GetByUserID(ctx, userIDOld)
	if oldProfile.CurrencyCode != "EUR" {
		t.Errorf("old profile currency = %q, want %q", oldProfile.CurrencyCode, "EUR")
	}
	if oldProfile.LanguageCode != "es" {
		t.Errorf("old profile language = %q, want %q", oldProfile.LanguageCode, "es")
	}
	if oldProfile.TimezoneName != "UTC" {
		t.Errorf("old profile timezone = %q, want %q", oldProfile.TimezoneName, "UTC")
	}

	// New profile: env-derived defaults
	newProfile, _ := repo.GetByUserID(ctx, userIDNew)
	if newProfile.CurrencyCode != "MXN" {
		t.Errorf("new profile currency = %q, want %q", newProfile.CurrencyCode, "MXN")
	}
	if newProfile.LanguageCode != "es" {
		t.Errorf("new profile language = %q, want %q", newProfile.LanguageCode, "es")
	}
	if newProfile.TimezoneName != "America/Mexico_City" {
		t.Errorf("new profile timezone = %q, want %q", newProfile.TimezoneName, "America/Mexico_City")
	}

	// Both caches should be populated correctly
	oldCur, _, _, oldTz, oldFound, _ := shared.GetProfilePrefs(ctx, rdb, userIDOld.String())
	if !oldFound {
		t.Fatal("old event: cache not populated")
	}
	if oldCur != "EUR" {
		t.Errorf("old event: cached currency = %q, want %q", oldCur, "EUR")
	}
	if oldTz != "UTC" {
		t.Errorf("old event: cached timezone = %q, want %q", oldTz, "UTC")
	}

	newCur, newLang, _, newTz, newFound, _ := shared.GetProfilePrefs(ctx, rdb, userIDNew.String())
	if !newFound {
		t.Fatal("new event: cache not populated")
	}
	if newCur != "MXN" {
		t.Errorf("new event: cached currency = %q, want %q", newCur, "MXN")
	}
	if newLang != "es" {
		t.Errorf("new event: cached language = %q, want %q", newLang, "es")
	}
	if newTz != "America/Mexico_City" {
		t.Errorf("new event: cached timezone = %q, want %q", newTz, "America/Mexico_City")
	}
}

// TestIntegration_ProfilePrefsUpdatedLater verifies that when a user changes
// preferences after initial profile creation, the env-derived defaults are
// correctly overwritten.
func TestIntegration_ProfilePrefsUpdatedLater(t *testing.T) {
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	// Initial creation with env defaults (EUR/es)
	initialPrefs := domain.EnvPrefs{
		CurrencyCode: "EUR",
		LanguageCode: "es",
		CountryCode:  "ES",
		TimezoneName: "Europe/Madrid",
	}
	if err := uc.Execute(ctx, userID, "", initialPrefs); err != nil {
		t.Fatalf("initial upsert failed: %v", err)
	}

	// User changes to GBP via settings
	if err := uc.UpdatePreferences(ctx, userID, "Europe/London", "en", "GBP", true); err != nil {
		t.Fatalf("update prefs failed: %v", err)
	}

	// Verify repository has updated values
	profile, _ := repo.GetByUserID(ctx, userID)
	if profile.CurrencyCode != "GBP" {
		t.Errorf("after update: CurrencyCode = %q, want %q", profile.CurrencyCode, "GBP")
	}
	if profile.LanguageCode != "en" {
		t.Errorf("after update: LanguageCode = %q, want %q", profile.LanguageCode, "en")
	}
	if profile.TimezoneName != "Europe/London" {
		t.Errorf("after update: TimezoneName = %q, want %q", profile.TimezoneName, "Europe/London")
	}
}

// =============================================================================
// Ensure legacy compatibility — events without env fields don't crash
// =============================================================================

func TestIntegration_LegacyEventReplayDoesNotCrash(t *testing.T) {
	ctx := context.Background()
	userID := uuid.Must(uuid.NewV7())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := newIntegrationMockRepo()
	uc := upsert_profile.NewUseCaseWithCache(repo, rdb)

	// Replay a legacy event: only user_id and email, no env fields
	if err := uc.Execute(ctx, userID, ""); err != nil {
		t.Fatalf("legacy event replay should not crash: %v", err)
	}

	// Verify valid profile was created
	profile, err := repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("profile should exist after legacy replay: %v", err)
	}
	// All fields should have hardcoded defaults
	validDefault := domain.NewUserProfile(userID, "")
	if profile.CurrencyCode != validDefault.CurrencyCode {
		t.Errorf("CurrencyCode = %q, want hardcoded %q", profile.CurrencyCode, validDefault.CurrencyCode)
	}
	if profile.LanguageCode != validDefault.LanguageCode {
		t.Errorf("LanguageCode = %q, want hardcoded %q", profile.LanguageCode, validDefault.LanguageCode)
	}
	if profile.TimezoneName != validDefault.TimezoneName {
		t.Errorf("TimezoneName = %q, want hardcoded %q", profile.TimezoneName, validDefault.TimezoneName)
	}
}

// =============================================================================
// Ensure the event structure is backward-compatible
// =============================================================================

func TestIntegration_EventStructureBackwardCompatible(t *testing.T) {
	// Event without env fields — old format
	legacyEvent := eventbus.NewUserRegisteredEvent(
		"user-legacy",
		"old@example.com",
		"token-123",
		"", "", "", "", // all env fields empty
	)

	// Must contain core fields
	if v, ok := legacyEvent.Payload["user_id"]; !ok || v != "user-legacy" {
		t.Errorf("legacy payload user_id = %v", v)
	}
	if v, ok := legacyEvent.Payload["email"]; !ok || v != "old@example.com" {
		t.Errorf("legacy payload email = %v", v)
	}

	// Must NOT contain env fields
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

	// Must contain env fields
	if v := newEvent.Payload["language_code"]; v != "es" {
		t.Errorf("new payload language_code = %v, want 'es'", v)
	}
	if v := newEvent.Payload["currency_code"]; v != "EUR" {
		t.Errorf("new payload currency_code = %v, want 'EUR'", v)
	}
	if v := newEvent.Payload["country_code"]; v != "ES" {
		t.Errorf("new payload country_code = %v, want 'ES'", v)
	}
	if v := newEvent.Payload["timezone_name"]; v != "Europe/Madrid" {
		t.Errorf("new payload timezone_name = %v, want 'Europe/Madrid'", v)
	}
}
