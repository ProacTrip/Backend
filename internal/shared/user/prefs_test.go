// Test del contrato compartido de preferencias de usuario — clave user:prefs:{userID} en Dragonfly.
// Verifica escritura/lectura del hash, cache miss, key format, y hash parcial.
package user_test

import (
	"testing"

	sharedUser "github.com/ProacTrip/Backend/internal/shared/user"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupPrefsTest(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

// ===================== GetProfilePrefs — cache hit =====================

func TestGetProfilePrefs_CacheHit(t *testing.T) {
	rdb, _ := setupPrefsTest(t)
	ctx := t.Context()
	userID := "550e8400-e29b-41d4-a716-446655440000"

	// Pre-popular el hash con el formato user:prefs:{userID}
	key := "user:prefs:" + userID
	fields := map[string]interface{}{
		"currency":     "ARS",
		"language":     "es",
		"country_code": "AR",
		"timezone":     "America/Argentina/Buenos_Aires",
	}
	if err := rdb.HSet(ctx, key, fields).Err(); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	prefs, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if prefs == nil {
		t.Fatal("expected non-nil prefs for pre-populated cache")
	}
	if prefs.Currency != "ARS" {
		t.Errorf("Currency = %q, want %q", prefs.Currency, "ARS")
	}
	if prefs.Language != "es" {
		t.Errorf("Language = %q, want %q", prefs.Language, "es")
	}
	if prefs.CountryCode != "AR" {
		t.Errorf("CountryCode = %q, want %q", prefs.CountryCode, "AR")
	}
	if prefs.Timezone != "America/Argentina/Buenos_Aires" {
		t.Errorf("Timezone = %q, want %q", prefs.Timezone, "America/Argentina/Buenos_Aires")
	}
}

// ===================== GetProfilePrefs — cache miss =====================

func TestGetProfilePrefs_CacheMiss(t *testing.T) {
	rdb, _ := setupPrefsTest(t)
	ctx := t.Context()
	userID := "660e8400-e29b-41d4-a716-446655440001"

	prefs, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if prefs != nil {
		t.Fatalf("expected nil prefs for cache miss, got %+v", prefs)
	}
}

// ===================== GetProfilePrefs — key format verification =====================

func TestGetProfilePrefs_KeyFormat(t *testing.T) {
	rdb, _ := setupPrefsTest(t)
	ctx := t.Context()
	userID := "770e8400-e29b-41d4-a716-446655440002"
	expectedKey := "user:prefs:" + userID

	// Poblar con el key esperado
	fields := map[string]interface{}{
		"currency": "USD",
	}
	if err := rdb.HSet(ctx, expectedKey, fields).Err(); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	prefs, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if prefs == nil {
		t.Fatal("expected found — key format must be user:prefs:{userID}")
	}
	if prefs.Currency != "USD" {
		t.Errorf("Currency = %q, want %q", prefs.Currency, "USD")
	}
}

// ===================== GetProfilePrefs — hash vacío =====================

func TestGetProfilePrefs_EmptyHash(t *testing.T) {
	rdb, _ := setupPrefsTest(t)
	ctx := t.Context()
	userID := "880e8400-e29b-41d4-a716-446655440003"

	// Crear una key vacía (sin campos) — debe ser cache miss
	key := "user:prefs:" + userID
	if err := rdb.HSet(ctx, key, map[string]interface{}{}).Err(); err != nil {
		// miniredis rechaza HSet con map vacío, así que usamos otra manera
		_ = rdb.Set(ctx, key, "", 0).Err() // crea key sin hash fields
		_ = rdb.Del(ctx, key).Err()         // borrar para simular vacío
	}

	prefs, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if prefs != nil {
		t.Fatal("expected nil prefs for empty/non-existent hash")
	}
}

// ===================== GetProfilePrefs — hash parcial =====================

func TestGetProfilePrefs_PartialHash(t *testing.T) {
	rdb, _ := setupPrefsTest(t)
	ctx := t.Context()
	userID := "990e8400-e29b-41d4-a716-446655440004"
	key := "user:prefs:" + userID

	// Solo setear algunos campos
	fields := map[string]interface{}{
		"language": "pt",
	}
	if err := rdb.HSet(ctx, key, fields).Err(); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	prefs, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if prefs == nil {
		t.Fatal("expected non-nil prefs when hash exists with partial fields")
	}
	if prefs.Language != "pt" {
		t.Errorf("Language = %q, want %q", prefs.Language, "pt")
	}
	if prefs.Currency != "" {
		t.Errorf("Currency = %q, want empty (field not set)", prefs.Currency)
	}
	if prefs.CountryCode != "" {
		t.Errorf("CountryCode = %q, want empty (field not set)", prefs.CountryCode)
	}
	if prefs.Timezone != "" {
		t.Errorf("Timezone = %q, want empty (field not set)", prefs.Timezone)
	}
}

// ===================== SetProfilePrefs — escribe todos los campos =====================

func TestSetProfilePrefs_WritesAllFields(t *testing.T) {
	rdb, _ := setupPrefsTest(t)
	ctx := t.Context()
	userID := "aa0e8400-e29b-41d4-a716-446655440005"

	prefs := &sharedUser.Prefs{
		Currency:    "ARS",
		Language:    "es",
		CountryCode: "AR",
		Timezone:    "America/Argentina/Buenos_Aires",
	}
	err := sharedUser.SetProfilePrefs(ctx, rdb, userID, prefs)
	if err != nil {
		t.Fatalf("SetProfilePrefs error: %v", err)
	}

	// Leer de vuelta via GetProfilePrefs
	got, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil prefs after SetProfilePrefs")
	}
	if got.Currency != "ARS" {
		t.Errorf("Currency = %q, want %q", got.Currency, "ARS")
	}
	if got.Language != "es" {
		t.Errorf("Language = %q, want %q", got.Language, "es")
	}
	if got.CountryCode != "AR" {
		t.Errorf("CountryCode = %q, want %q", got.CountryCode, "AR")
	}
	if got.Timezone != "America/Argentina/Buenos_Aires" {
		t.Errorf("Timezone = %q, want %q", got.Timezone, "America/Argentina/Buenos_Aires")
	}
}

// ===================== SetProfilePrefs — nil prefs =====================

func TestSetProfilePrefs_NilPrefs(t *testing.T) {
	rdb, _ := setupPrefsTest(t)
	ctx := t.Context()
	userID := "bb0e8400-e29b-41d4-a716-446655440006"

	err := sharedUser.SetProfilePrefs(ctx, rdb, userID, nil)
	if err != nil {
		t.Fatalf("SetProfilePrefs with nil prefs should not error: %v", err)
	}

	// No debería haber escrito nada
	got, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil prefs when nothing was stored")
	}
}

// ===================== SetProfilePrefs — campos vacíos =====================

func TestSetProfilePrefs_EmptyFields(t *testing.T) {
	rdb, _ := setupPrefsTest(t)
	ctx := t.Context()
	userID := "cc0e8400-e29b-41d4-a716-446655440007"

	prefs := &sharedUser.Prefs{
		Currency: "BRL",
		// Language, CountryCode, Timezone vacíos
	}
	err := sharedUser.SetProfilePrefs(ctx, rdb, userID, prefs)
	if err != nil {
		t.Fatalf("SetProfilePrefs error: %v", err)
	}

	got, err := sharedUser.GetProfilePrefs(ctx, rdb, userID)
	if err != nil {
		t.Fatalf("GetProfilePrefs error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil prefs (hash exists with at least one field)")
	}
	if got.Currency != "BRL" {
		t.Errorf("Currency = %q, want %q", got.Currency, "BRL")
	}
	if got.Language != "" {
		t.Errorf("Language = %q, want empty", got.Language)
	}
}
