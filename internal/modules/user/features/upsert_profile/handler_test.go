// Tests del usecase upsert_profile desde perspectiva del consumer.
// UpsertProfile no tiene handler HTTP — es llamado internamente por el
// consumer de eventos UserRegistered. Estos tests verifican el contrato
// que el consumer espera (U-SPEC-011 excepción documentada).
package upsert_profile

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Helper
// =============================================================================

func setupMiniRedisHandler(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

// =============================================================================
// Test: Consumer llama Execute() con evento UserRegistered completo
// =============================================================================

func TestHandler_ConsumerCall_RegistrationEvent(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	email := "user@example.com"

	var savedProfile *domain.UserProfile
	repo := &mockProfileRepo{
		upsertProfileFn: func(_ context.Context, p *domain.UserProfile) error {
			savedProfile = p
			return nil
		},
	}

	uc := NewUseCase(repo)

	// Simula lo que el consumer hace:
	//  1. Extrae user_id y email del evento UserRegistered
	//  2. Extrae envPrefs del payload
	//  3. Llama uc.Execute(...)
	envPrefs := domain.EnvPrefs{
		LanguageCode: "pt",
		CurrencyCode: "BRL",
		CountryCode:  "BR",
		TimezoneName: "America/Sao_Paulo",
	}

	if err := uc.Execute(t.Context(), userID, email, envPrefs); err != nil {
		t.Fatalf("consumer call failed: %v", err)
	}

	if savedProfile == nil {
		t.Fatal("Execute did not call UpsertProfile")
	}
	if savedProfile.UserID != userID {
		t.Errorf("UserID = %s, want %s", savedProfile.UserID, userID)
	}
	if savedProfile.TimezoneName != "America/Sao_Paulo" {
		t.Errorf("TimezoneName = %q, want %q", savedProfile.TimezoneName, "America/Sao_Paulo")
	}
	if savedProfile.LanguageCode != "pt" {
		t.Errorf("LanguageCode = %q, want %q", savedProfile.LanguageCode, "pt")
	}
	if savedProfile.CurrencyCode != "BRL" {
		t.Errorf("CurrencyCode = %q, want %q", savedProfile.CurrencyCode, "BRL")
	}
}

// =============================================================================
// Test: Consumer llama con evento Legacy (sin env fields)
// =============================================================================

func TestHandler_ConsumerCall_LegacyEventNoEnv(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	var savedProfile *domain.UserProfile
	repo := &mockProfileRepo{
		upsertProfileFn: func(_ context.Context, p *domain.UserProfile) error {
			savedProfile = p
			return nil
		},
	}

	uc := NewUseCase(repo)

	// Legacy event: no env fields
	if err := uc.Execute(t.Context(), userID, ""); err != nil {
		t.Fatalf("legacy event call failed: %v", err)
	}

	if savedProfile == nil {
		t.Fatal("Execute did not call UpsertProfile")
	}
	// Should use hardcoded defaults
	if savedProfile.CurrencyCode != domain.DefaultCurrency {
		t.Errorf("CurrencyCode = %q, want default %q", savedProfile.CurrencyCode, domain.DefaultCurrency)
	}
	if savedProfile.LanguageCode != domain.DefaultLanguage {
		t.Errorf("LanguageCode = %q, want default %q", savedProfile.LanguageCode, domain.DefaultLanguage)
	}
	if savedProfile.TimezoneName != domain.DefaultTimezone {
		t.Errorf("TimezoneName = %q, want default %q", savedProfile.TimezoneName, domain.DefaultTimezone)
	}
}

// =============================================================================
// Test: Consumer llama con cache (via NewUseCaseWithCache)
// =============================================================================

func TestHandler_ConsumerCall_WithCache(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	rdb, _ := setupMiniRedisHandler(t)

	uc := NewUseCaseWithCache(&mockProfileRepo{}, rdb)

	// Simular consumer con cache
	envPrefs := domain.EnvPrefs{
		CurrencyCode: "USD",
		LanguageCode: "en",
		TimezoneName: "America/New_York",
	}

	if err := uc.Execute(t.Context(), userID, "", envPrefs); err != nil {
		t.Fatalf("consumer call with cache failed: %v", err)
	}

	// Verify cache populated (same verification pattern que el consumer usa en tests)
	key := "user:prefs:" + userID.String()
	exists := rdb.Exists(t.Context(), key).Val()
	if exists == 0 {
		t.Fatal("cache key not created — consumer expects cache after upsert")
	}
}
