// RED phase — tests for feature_limits usecase.
// These reference types and functions that do NOT exist yet.
// They MUST fail to compile initially.
package feature_limits_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	featurelimits "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/feature_limits"
)

// =============================================================================
// Stubs
// =============================================================================

type stubFeatureLimitRepo struct {
	getUserLimits    func(ctx context.Context, userID uuid.UUID) ([]featurelimits.FeatureLimitRow, error)
	setUserLimit     func(ctx context.Context, userID uuid.UUID, featureKey string, limitValue *int, window string) (bool, error)
	deleteUserLimit  func(ctx context.Context, userID uuid.UUID, featureKey string) error
	getUserLimitVal  func(ctx context.Context, userID uuid.UUID, featureKey string) (*int, error)
	getRoleDefaultVal func(ctx context.Context, roleID uuid.UUID, featureKey string) (*int, error)
}

func (s *stubFeatureLimitRepo) GetUserLimits(ctx context.Context, userID uuid.UUID) ([]featurelimits.FeatureLimitRow, error) {
	return s.getUserLimits(ctx, userID)
}
func (s *stubFeatureLimitRepo) SetUserLimit(ctx context.Context, userID uuid.UUID, featureKey string, limitValue *int, window string) (bool, error) {
	return s.setUserLimit(ctx, userID, featureKey, limitValue, window)
}
func (s *stubFeatureLimitRepo) DeleteUserLimit(ctx context.Context, userID uuid.UUID, featureKey string) error {
	return s.deleteUserLimit(ctx, userID, featureKey)
}
func (s *stubFeatureLimitRepo) GetUserLimitValue(ctx context.Context, userID uuid.UUID, featureKey string) (*int, error) {
	return s.getUserLimitVal(ctx, userID, featureKey)
}
func (s *stubFeatureLimitRepo) GetRoleDefaultValue(ctx context.Context, roleID uuid.UUID, featureKey string) (*int, error) {
	return s.getRoleDefaultVal(ctx, roleID, featureKey)
}

func newUseCase(repo *stubFeatureLimitRepo) *featurelimits.UseCase {
	return featurelimits.NewUseCase(repo)
}

// =============================================================================
// Fixtures
// =============================================================================

func ptr(i int) *int { return &i }

// =============================================================================
// Tests — CRUD User Limits
// =============================================================================

// TestGetUserLimits_Success retorna lista de límites del usuario.
func TestGetUserLimits_Success(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		getUserLimits: func(ctx context.Context, uid uuid.UUID) ([]featurelimits.FeatureLimitRow, error) {
			return []featurelimits.FeatureLimitRow{
				{FeatureKey: "projects", LimitValue: ptr(5), Window: "month"},
				{FeatureKey: "reports", LimitValue: nil, Window: "day"},
			}, nil
		},
	}
	uc := newUseCase(repo)

	cmd := featurelimits.GetUserLimitsCommand{UserID: userID}
	resp, err := uc.GetUserLimits(ctx, cmd)
	if err != nil {
		t.Fatalf("GetUserLimits() unexpected error: %v", err)
	}
	if len(resp.Limits) != 2 {
		t.Fatalf("expected 2 limits, got %d", len(resp.Limits))
	}
	if resp.Limits[0].FeatureKey != "projects" {
		t.Errorf("limit[0].FeatureKey = %s, expected projects", resp.Limits[0].FeatureKey)
	}
	if resp.Limits[1].LimitValue != nil {
		t.Errorf("limit[1].LimitValue should be nil (unlimited), got %d", *resp.Limits[1].LimitValue)
	}
}

// TestSetUserLimit_Success crea un límite de usuario.
func TestSetUserLimit_Success(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		setUserLimit: func(ctx context.Context, uid uuid.UUID, fk string, lv *int, w string) (bool, error) {
			return true, nil // isCreated=true
		},
	}
	uc := newUseCase(repo)

	cmd := featurelimits.SetUserLimitCommand{
		UserID:     userID,
		FeatureKey: "projects",
		LimitValue: ptr(5),
		Window:     "month",
	}
	resp, _, err := uc.SetUserLimit(ctx, cmd)
	if err != nil {
		t.Fatalf("SetUserLimit() unexpected error: %v", err)
	}
	if resp.FeatureKey != "projects" {
		t.Errorf("FeatureKey = %s, expected projects", resp.FeatureKey)
	}
	if *resp.LimitValue != 5 {
		t.Errorf("LimitValue = %d, expected 5", *resp.LimitValue)
	}
}

// TestSetUserLimit_Unlimited crea un límite NULL (ilimitado).
func TestSetUserLimit_Unlimited(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		setUserLimit: func(ctx context.Context, uid uuid.UUID, fk string, lv *int, w string) (bool, error) {
			return true, nil // isCreated=true
		},
	}
	uc := newUseCase(repo)

	cmd := featurelimits.SetUserLimitCommand{
		UserID:     userID,
		FeatureKey: "projects",
		LimitValue: nil, // unlimited
		Window:     "",
	}
	resp, _, err := uc.SetUserLimit(ctx, cmd)
	if err != nil {
		t.Fatalf("SetUserLimit() unexpected error: %v", err)
	}
	if resp.LimitValue != nil {
		t.Errorf("LimitValue should be nil (unlimited), got %d", *resp.LimitValue)
	}
}

// TestSetUserLimit_Blocked crea límite con valor 0 (bloqueado).
func TestSetUserLimit_Blocked(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		setUserLimit: func(ctx context.Context, uid uuid.UUID, fk string, lv *int, w string) (bool, error) {
			return true, nil // isCreated=true
		},
	}
	uc := newUseCase(repo)

	cmd := featurelimits.SetUserLimitCommand{
		UserID:     userID,
		FeatureKey: "projects",
		LimitValue: ptr(0), // bloqueado
		Window:     "",
	}
	resp, _, err := uc.SetUserLimit(ctx, cmd)
	if err != nil {
		t.Fatalf("SetUserLimit() unexpected error: %v", err)
	}
	if *resp.LimitValue != 0 {
		t.Errorf("LimitValue = %d, expected 0", *resp.LimitValue)
	}
}

// TestSetUserLimit_DuplicateKey retorna 409 conflict.
func TestSetUserLimit_DuplicateKey(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		setUserLimit: func(ctx context.Context, uid uuid.UUID, fk string, lv *int, w string) (bool, error) {
			return true, domain.ErrFeatureLimitAlreadyExists
		},
	}
	uc := newUseCase(repo)

	cmd := featurelimits.SetUserLimitCommand{
		UserID:     userID,
		FeatureKey: "projects",
		LimitValue: ptr(5),
	}
	_, _, err := uc.SetUserLimit(ctx, cmd)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !errors.Is(err, domain.ErrFeatureLimitAlreadyExists) {
		t.Errorf("expected ErrFeatureLimitAlreadyExists, got %v", err)
	}
}

// TestDeleteUserLimit_Success elimina un límite de usuario.
func TestDeleteUserLimit_Success(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		deleteUserLimit: func(ctx context.Context, uid uuid.UUID, fk string) error {
			return nil
		},
	}
	uc := newUseCase(repo)

	cmd := featurelimits.DeleteUserLimitCommand{
		UserID:     userID,
		FeatureKey: "projects",
	}
	err := uc.DeleteUserLimit(ctx, cmd)
	if err != nil {
		t.Fatalf("DeleteUserLimit() unexpected error: %v", err)
	}
}

// TestDeleteUserLimit_NotFound retorna error.
func TestDeleteUserLimit_NotFound(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		deleteUserLimit: func(ctx context.Context, uid uuid.UUID, fk string) error {
			return domain.ErrFeatureLimitNotFound
		},
	}
	uc := newUseCase(repo)

	cmd := featurelimits.DeleteUserLimitCommand{
		UserID:     userID,
		FeatureKey: "nonexistent",
	}
	err := uc.DeleteUserLimit(ctx, cmd)
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
	if !errors.Is(err, domain.ErrFeatureLimitNotFound) {
		t.Errorf("expected ErrFeatureLimitNotFound, got %v", err)
	}
}

// =============================================================================
// Tests — FeatureLimitService: GetEffectiveLimit
// =============================================================================

// TestGetEffectiveLimit_UserOverride gana el límite de usuario sobre el del rol.
func TestGetEffectiveLimit_UserOverride(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		getUserLimitVal: func(ctx context.Context, uid uuid.UUID, fk string) (*int, error) {
			return ptr(5), nil
		},
		getRoleDefaultVal: func(ctx context.Context, rid uuid.UUID, fk string) (*int, error) {
			return ptr(10), nil
		},
	}
	svc := featurelimits.NewFeatureLimitService(repo)

	limit, err := svc.GetEffectiveLimit(ctx, userID, roleID, "projects")
	if err != nil {
		t.Fatalf("GetEffectiveLimit() unexpected error: %v", err)
	}
	if limit != 5 {
		t.Errorf("expected limit=5 (user override), got %d", limit)
	}
}

// TestGetEffectiveLimit_RoleDefault usa el default del rol cuando no hay límite de usuario.
func TestGetEffectiveLimit_RoleDefault(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		getUserLimitVal: func(ctx context.Context, uid uuid.UUID, fk string) (*int, error) {
			return nil, nil // sin límite de usuario
		},
		getRoleDefaultVal: func(ctx context.Context, rid uuid.UUID, fk string) (*int, error) {
			return ptr(3), nil
		},
	}
	svc := featurelimits.NewFeatureLimitService(repo)

	limit, err := svc.GetEffectiveLimit(ctx, userID, roleID, "projects")
	if err != nil {
		t.Fatalf("GetEffectiveLimit() unexpected error: %v", err)
	}
	if limit != 3 {
		t.Errorf("expected limit=3 (role default), got %d", limit)
	}
}

// TestGetEffectiveLimit_Unlimited retorna MaxInt cuando no hay límites.
func TestGetEffectiveLimit_Unlimited(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		getUserLimitVal: func(ctx context.Context, uid uuid.UUID, fk string) (*int, error) {
			return nil, nil
		},
		getRoleDefaultVal: func(ctx context.Context, rid uuid.UUID, fk string) (*int, error) {
			return nil, nil
		},
	}
	svc := featurelimits.NewFeatureLimitService(repo)

	limit, err := svc.GetEffectiveLimit(ctx, userID, roleID, "projects")
	if err != nil {
		t.Fatalf("GetEffectiveLimit() unexpected error: %v", err)
	}
	if limit != math.MaxInt {
		t.Errorf("expected math.MaxInt (unlimited), got %d", limit)
	}
}

// =============================================================================
// Tests — FeatureLimitService: CanConsume
// =============================================================================

// TestCanConsume_Allowed cuando el límite efectivo > uso actual.
func TestCanConsume_Allowed(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		getUserLimitVal: func(ctx context.Context, uid uuid.UUID, fk string) (*int, error) {
			return ptr(5), nil
		},
		getRoleDefaultVal: func(ctx context.Context, rid uuid.UUID, fk string) (*int, error) {
			return nil, nil
		},
	}
	svc := featurelimits.NewFeatureLimitService(repo)

	allowed, err := svc.CanConsume(ctx, userID, roleID, "projects")
	if err != nil {
		t.Fatalf("CanConsume() unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected CanConsume=true (limit 5, usage 0)" )
	}
}

// TestCanConsume_Blocked rechaza cuando el límite efectivo es 0.
func TestCanConsume_Blocked(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		getUserLimitVal: func(ctx context.Context, uid uuid.UUID, fk string) (*int, error) {
			return ptr(0), nil // 0 = bloqueado
		},
		getRoleDefaultVal: func(ctx context.Context, rid uuid.UUID, fk string) (*int, error) {
			return nil, nil
		},
	}
	svc := featurelimits.NewFeatureLimitService(repo)

	allowed, err := svc.CanConsume(ctx, userID, roleID, "projects")
	if err != nil {
		t.Fatalf("CanConsume() unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected CanConsume=false when limit=0 (blocked)")
	}
}

// TestCanConsume_Unlimited siempre permite cuando no hay límite.
func TestCanConsume_Unlimited(t *testing.T) {
	ctx := t.Context()
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	repo := &stubFeatureLimitRepo{
		getUserLimitVal: func(ctx context.Context, uid uuid.UUID, fk string) (*int, error) {
			return nil, nil
		},
		getRoleDefaultVal: func(ctx context.Context, rid uuid.UUID, fk string) (*int, error) {
			return nil, nil
		},
	}
	svc := featurelimits.NewFeatureLimitService(repo)

	allowed, err := svc.CanConsume(ctx, userID, roleID, "projects")
	if err != nil {
		t.Fatalf("CanConsume() unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected CanConsume=true when unlimited")
	}
}

// =============================================================================
// Tests — FeatureLimitService: Consume (stub)
// =============================================================================

// TestConsume_NotImplemented retorna ErrNotImplemented.
func TestConsume_NotImplemented(t *testing.T) {
	ctx := t.Context()

	repo := &stubFeatureLimitRepo{}
	svc := featurelimits.NewFeatureLimitService(repo)

	err := svc.Consume(ctx, uuid.Nil, "projects")
	if err == nil {
		t.Fatal("expected ErrNotImplemented, got nil")
	}
	if !errors.Is(err, domain.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

// =============================================================================
// Tests — Validación de comandos
// =============================================================================

// TestSetUserLimitCommand_Validate_EmptyFeatureKey rechaza feature_key vacío.
func TestSetUserLimitCommand_Validate_EmptyFeatureKey(t *testing.T) {
	cmd := featurelimits.SetUserLimitCommand{
		UserID:     uuid.Must(uuid.NewV7()),
		FeatureKey: "",
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for empty feature key, got nil")
	}
}

// TestSetUserLimitCommand_Validate_NilUserID rechaza uuid.Nil.
func TestSetUserLimitCommand_Validate_NilUserID(t *testing.T) {
	cmd := featurelimits.SetUserLimitCommand{
		UserID:     uuid.Nil,
		FeatureKey: "projects",
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for nil userID, got nil")
	}
}
