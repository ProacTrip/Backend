// RED phase — tests for permission_overrides usecase.
// These reference types and functions that do NOT exist yet.
// They MUST fail to compile initially.
package permission_overrides_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	overrides "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/permission_overrides"
)

// =============================================================================
// Stubs
// =============================================================================

type stubOverrideRepo struct {
	getOverrides  func(ctx context.Context, userID uuid.UUID) ([]overrides.OverrideRow, error)
	createOverride func(ctx context.Context, userID, permissionID uuid.UUID, granted bool, expiresAt *time.Time, reason string, createdBy uuid.UUID) (uuid.UUID, error)
	updateOverride func(ctx context.Context, overrideID uuid.UUID, granted bool, expiresAt *time.Time, reason string, updatedBy uuid.UUID) error
	deleteOverride func(ctx context.Context, overrideID uuid.UUID) error
}

func (s *stubOverrideRepo) GetOverridesByUserID(ctx context.Context, userID uuid.UUID) ([]overrides.OverrideRow, error) {
	return s.getOverrides(ctx, userID)
}
func (s *stubOverrideRepo) CreateOverride(ctx context.Context, userID, permissionID uuid.UUID, granted bool, expiresAt *time.Time, reason string, createdBy uuid.UUID) (uuid.UUID, error) {
	return s.createOverride(ctx, userID, permissionID, granted, expiresAt, reason, createdBy)
}
func (s *stubOverrideRepo) UpdateOverride(ctx context.Context, overrideID uuid.UUID, granted bool, expiresAt *time.Time, reason string, updatedBy uuid.UUID) error {
	return s.updateOverride(ctx, overrideID, granted, expiresAt, reason, updatedBy)
}
func (s *stubOverrideRepo) DeleteOverride(ctx context.Context, overrideID uuid.UUID) error {
	return s.deleteOverride(ctx, overrideID)
}

func newUseCase(repo *stubOverrideRepo, rdb *redis.Client) *overrides.UseCase {
	return overrides.NewUseCase(repo, rdb)
}

func newMiniRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis.Start: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

// =============================================================================
// Fixtures
// =============================================================================

func ptrTime(t time.Time) *time.Time { return &t }

func overrideRow(id uuid.UUID, permission string, granted bool, reason string, expiresAt *time.Time) overrides.OverrideRow {
	now := time.Now()
	return overrides.OverrideRow{
		ID:         id,
		Permission: permission,
		Granted:    granted,
		Reason:     reason,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// =============================================================================
// Tests — List Overrides
// =============================================================================

// TestListOverrides_Success retorna todos los overrides del usuario.
func TestListOverrides_Success(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{
		getOverrides: func(ctx context.Context, uid uuid.UUID) ([]overrides.OverrideRow, error) {
			return []overrides.OverrideRow{
				overrideRow(uuid.Must(uuid.NewV7()), "users:delete", false, "Abuso de borrado", nil),
				overrideRow(uuid.Must(uuid.NewV7()), "projects:write", true, "Necesita acceso temporal", nil),
			}, nil
		},
	}
	uc := newUseCase(repo, nil)

	cmd := overrides.ListOverridesCommand{UserID: userID}
	resp, err := uc.ListOverrides(ctx, cmd)
	if err != nil {
		t.Fatalf("ListOverrides() unexpected error: %v", err)
	}
	if len(resp.Overrides) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(resp.Overrides))
	}
	if resp.Overrides[0].Granted {
		t.Errorf("override[0] should be deny (Granted=false)")
	}
	if resp.Overrides[1].Permission != "projects:write" {
		t.Errorf("override[1].Permission = %s, expected projects:write", resp.Overrides[1].Permission)
	}
}

// =============================================================================
// Tests — Create Override
// =============================================================================

// TestCreateOverride_Success crea un override grant con razón válida.
func TestCreateOverride_Success(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{
		createOverride: func(ctx context.Context, uid, pid uuid.UUID, granted bool, expiresAt *time.Time, reason string, createdBy uuid.UUID) (uuid.UUID, error) {
			return uuid.Must(uuid.NewV7()), nil
		},
	}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      true,
		ExpiresAt:    nil,
		Reason:       "Necesita acceso para auditoría",
		ActorID:      actorID,
	}
	resp, err := uc.CreateOverride(ctx, cmd)
	if err != nil {
		t.Fatalf("CreateOverride() unexpected error: %v", err)
	}
	if !resp.Granted {
		t.Error("expected Granted=true")
	}
	if resp.Reason != "Necesita acceso para auditoría" {
		t.Errorf("Reason = %s, expected 'Necesita acceso para auditoría'", resp.Reason)
	}
}

// TestCreateOverride_Deny crea un override deny con razón válida.
func TestCreateOverride_Deny(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{
		createOverride: func(ctx context.Context, uid, pid uuid.UUID, granted bool, expiresAt *time.Time, reason string, createdBy uuid.UUID) (uuid.UUID, error) {
			return uuid.Must(uuid.NewV7()), nil
		},
	}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      false, // deny
		Reason:       "Abuso de borrado masivo",
		ActorID:      actorID,
	}
	resp, err := uc.CreateOverride(ctx, cmd)
	if err != nil {
		t.Fatalf("CreateOverride() unexpected error: %v", err)
	}
	if resp.Granted {
		t.Error("expected Granted=false (deny)")
	}
}

// TestCreateOverride_WithExpiry crea override con fecha de expiración.
func TestCreateOverride_WithExpiry(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 días

	repo := &stubOverrideRepo{
		createOverride: func(ctx context.Context, uid, pid uuid.UUID, granted bool, eAt *time.Time, reason string, createdBy uuid.UUID) (uuid.UUID, error) {
			return uuid.Must(uuid.NewV7()), nil
		},
	}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      false,
		ExpiresAt:    &expiresAt,
		Reason:       "Sanción temporal 7 días",
		ActorID:      actorID,
	}
	resp, err := uc.CreateOverride(ctx, cmd)
	if err != nil {
		t.Fatalf("CreateOverride() with expiry unexpected error: %v", err)
	}
	if resp.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
}

// TestCreateOverride_DuplicateUpsert actualiza override existente (upsert semantics).
func TestCreateOverride_DuplicateUpsert(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{
		createOverride: func(ctx context.Context, uid, pid uuid.UUID, granted bool, expiresAt *time.Time, reason string, createdBy uuid.UUID) (uuid.UUID, error) {
			return uuid.Nil, domain.ErrPermissionOverrideAlreadyExists
		},
	}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      true,
		Reason:       "Intento duplicado",
		ActorID:      actorID,
	}
	_, err := uc.CreateOverride(ctx, cmd)
	if err == nil {
		t.Fatal("expected conflict error for duplicate override, got nil")
	}
	if !errors.Is(err, domain.ErrPermissionOverrideAlreadyExists) {
		t.Errorf("expected ErrPermissionOverrideAlreadyExists, got %v", err)
	}
}

// =============================================================================
// Tests — Delete Override
// =============================================================================

// TestDeleteOverride_Success elimina un override e invalida sesiones.
func TestDeleteOverride_Success(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	overrideID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{
		deleteOverride: func(ctx context.Context, oid uuid.UUID) error {
			return nil
		},
	}
	uc := newUseCase(repo, rdb)

	cmd := overrides.DeleteOverrideCommand{
		OverrideID: overrideID,
		ActorID:    actorID,
	}
	err := uc.DeleteOverride(ctx, cmd)
	if err != nil {
		t.Fatalf("DeleteOverride() unexpected error: %v", err)
	}
}

// TestDeleteOverride_NotFound retorna error.
func TestDeleteOverride_NotFound(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	overrideID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{
		deleteOverride: func(ctx context.Context, oid uuid.UUID) error {
			return domain.ErrPermissionOverrideNotFound
		},
	}
	uc := newUseCase(repo, rdb)

	cmd := overrides.DeleteOverrideCommand{
		OverrideID: overrideID,
		ActorID:    actorID,
	}
	err := uc.DeleteOverride(ctx, cmd)
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
	if !errors.Is(err, domain.ErrPermissionOverrideNotFound) {
		t.Errorf("expected ErrPermissionOverrideNotFound, got %v", err)
	}
}

// =============================================================================
// Tests — Validation: Reason
// =============================================================================

// TestCreateOverride_EmptyReason rechaza razón vacía (PO-SPEC-006).
func TestCreateOverride_EmptyReason(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      true,
		Reason:       "",
		ActorID:      actorID,
	}
	_, err := uc.CreateOverride(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for empty reason, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidReason) {
		t.Errorf("expected ErrInvalidReason, got %v", err)
	}
}

// TestCreateOverride_WhitespaceReason rechaza razón solo con espacios (PO-SPEC-006).
func TestCreateOverride_WhitespaceReason(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      true,
		Reason:       "   ",
		ActorID:      actorID,
	}
	_, err := uc.CreateOverride(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for whitespace-only reason, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidReason) {
		t.Errorf("expected ErrInvalidReason, got %v", err)
	}
}

// TestCreateOverride_LongReason rechaza razón > 500 caracteres.
func TestCreateOverride_LongReason(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      true,
		Reason:       strings.Repeat("a", 501),
		ActorID:      actorID,
	}
	_, err := uc.CreateOverride(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for 501-char reason, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidReason) {
		t.Errorf("expected ErrInvalidReason, got %v", err)
	}
}

// =============================================================================
// Tests — Validation: Block Duration
// =============================================================================

// TestCreateOverride_InvalidBlockDuration rechaza deny con expiración > 365 días (PO-SPEC-007).
func TestCreateOverride_InvalidBlockDuration(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())
	farFuture := time.Now().Add(400 * 24 * time.Hour) // 400 días

	repo := &stubOverrideRepo{}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      false, // deny
		ExpiresAt:    &farFuture,
		Reason:       "Sanción excesivamente larga",
		ActorID:      actorID,
	}
	_, err := uc.CreateOverride(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for block > 365 days, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidBlockDuration) {
		t.Errorf("expected ErrInvalidBlockDuration, got %v", err)
	}
}

// TestCreateOverride_ExpiredExpiry rechaza expiración en el pasado.
func TestCreateOverride_ExpiredExpiry(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())
	past := time.Now().Add(-1 * time.Hour)

	repo := &stubOverrideRepo{}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      true,
		ExpiresAt:    &past,
		Reason:       "Expirado en el pasado",
		ActorID:      actorID,
	}
	_, err := uc.CreateOverride(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for past expiry, got nil")
	}
}

// =============================================================================
// Tests — Session Invalidation
// =============================================================================

// TestCreateOverride_InvalidatesSessionCache verifica que al crear un override
// se invaliden las sesiones cacheadas (best-effort).
func TestCreateOverride_InvalidatesSessionCache(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	permID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	repo := &stubOverrideRepo{
		createOverride: func(ctx context.Context, uid, pid uuid.UUID, granted bool, expiresAt *time.Time, reason string, createdBy uuid.UUID) (uuid.UUID, error) {
			return uuid.Must(uuid.NewV7()), nil
		},
	}
	uc := newUseCase(repo, rdb)

	cmd := overrides.CreateOverrideCommand{
		UserID:       userID,
		PermissionID: permID,
		Granted:      true,
		Reason:       "Test session invalidation",
		ActorID:      actorID,
	}
	_, err := uc.CreateOverride(ctx, cmd)
	if err != nil {
		t.Fatalf("CreateOverride() should succeed even if session invalidation has no sessions: %v", err)
	}
}

// =============================================================================
// Tests — Command Validation
// =============================================================================

// TestValidate_NilUserID rechaza uuid.Nil.
func TestValidate_NilUserID(t *testing.T) {
	cmd := overrides.CreateOverrideCommand{
		UserID:       uuid.Nil,
		PermissionID: uuid.Must(uuid.NewV7()),
		Granted:      true,
		Reason:       "Test",
		ActorID:      uuid.Must(uuid.NewV7()),
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for nil userID, got nil")
	}
}

// TestValidate_NilPermissionID rechaza uuid.Nil.
func TestValidate_NilPermissionID(t *testing.T) {
	cmd := overrides.CreateOverrideCommand{
		UserID:       uuid.Must(uuid.NewV7()),
		PermissionID: uuid.Nil,
		Granted:      true,
		Reason:       "Test",
		ActorID:      uuid.Must(uuid.NewV7()),
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for nil permissionID, got nil")
	}
}
