// FeatureLimitService — servicio de dominio para resolución de límites efectivos.
// FL-SPEC-003: GetEffectiveLimit resuelve user_limit → role_default → unlimited.
// FL-SPEC-004: CanConsume verifica si el uso actual está dentro del límite.
// FL-SPEC-005: Consume es un contrato de interfaz futuro (stub).
package feature_limits

import (
	"context"
	"math"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// FeatureLimitResolutionRepo — puerto de resolución (subconjunto de FeatureLimitRepo)
// =============================================================================

// FeatureLimitResolutionRepo expone solo los métodos necesarios para resolver
// límites efectivos. Es un subconjunto de FeatureLimitRepo.
type FeatureLimitResolutionRepo interface {
	GetUserLimitValue(ctx context.Context, userID uuid.UUID, featureKey string) (*int, error)
	GetRoleDefaultValue(ctx context.Context, roleID uuid.UUID, featureKey string) (*int, error)
}

// =============================================================================
// FeatureLimitService — interfaz del servicio
// =============================================================================

// FeatureLimitService resuelve límites efectivos y controla consumo de features.
// Toda la lógica de límites pasa por este servicio — no se dispersa en usecases.
type FeatureLimitService interface {
	// GetEffectiveLimit retorna el límite efectivo para (user, feature).
	// Resolución: user_limit → role_default → math.MaxInt (ilimitado).
	GetEffectiveLimit(ctx context.Context, userID, roleID uuid.UUID, featureKey string) (int, error)

	// CanConsume verifica si se puede consumir una unidad del feature.
	// Retorna false si el límite efectivo es 0 (bloqueado).
	// Retorna true si es ilimitado (math.MaxInt).
	CanConsume(ctx context.Context, userID, roleID uuid.UUID, featureKey string) (bool, error)

	// FUTURE: implementar cuando módulos consumidores existan.
	// Consume consume atómicamente una unidad del feature (check + decrement).
	Consume(ctx context.Context, userID uuid.UUID, featureKey string) error
}

// =============================================================================
// DefaultFeatureLimitService — implementación concreta
// =============================================================================

// DefaultFeatureLimitService implementa FeatureLimitService usando el repo.
type DefaultFeatureLimitService struct {
	repo FeatureLimitResolutionRepo
}

// NewFeatureLimitService crea un nuevo DefaultFeatureLimitService.
func NewFeatureLimitService(repo FeatureLimitResolutionRepo) *DefaultFeatureLimitService {
	return &DefaultFeatureLimitService{repo: repo}
}

// =============================================================================
// GetEffectiveLimit
// =============================================================================

// GetEffectiveLimit resuelve el límite efectivo con la semántica contractual:
//   - NULL (nil) → ilimitado (math.MaxInt)
//   - 0 → bloqueado
//   - >0 → cuota
//
// Resolución: IF user_limit NOT NULL → user_limit
//
//	ELSE IF role_default NOT NULL → role_default
//	ELSE → math.MaxInt (ilimitado).
func (s *DefaultFeatureLimitService) GetEffectiveLimit(
	ctx context.Context,
	userID, roleID uuid.UUID,
	featureKey string,
) (int, error) {
	// 1. Verificar límite de usuario
	userLimit, err := s.repo.GetUserLimitValue(ctx, userID, featureKey)
	if err != nil {
		return 0, err
	}
	if userLimit != nil {
		return *userLimit, nil
	}

	// 2. Fallback a default del rol
	roleLimit, err := s.repo.GetRoleDefaultValue(ctx, roleID, featureKey)
	if err != nil {
		return 0, err
	}
	if roleLimit != nil {
		return *roleLimit, nil
	}

	// 3. Sin límites → ilimitado
	return math.MaxInt, nil
}

// =============================================================================
// CanConsume
// =============================================================================

// CanConsume verifica si el usuario puede consumir una unidad del feature.
// La lógica de uso actual es pluggable (interfaz futura). Por ahora, asume
// uso=0 y solo verifica el límite efectivo contra 0 (bloqueado) y MaxInt (ilimitado).
//
// FUTURE: integrar UsageDataSource para consultar uso real.
func (s *DefaultFeatureLimitService) CanConsume(
	ctx context.Context,
	userID, roleID uuid.UUID,
	featureKey string,
) (bool, error) {
	limit, err := s.GetEffectiveLimit(ctx, userID, roleID, featureKey)
	if err != nil {
		return false, err
	}

	// Bloqueado explícitamente (limit == 0)
	if limit == 0 {
		return false, nil
	}

	// Ilimitado o con cuota > 0 → permitido
	// FUTURE: cuando haya UsageDataSource, verificar currentUsage < limit
	return true, nil
}

// =============================================================================
// Consume — stub futuro
// =============================================================================

// Consume es un stub que retorna ErrNotImplemented.
// FUTURE: implementar cuando módulos consumidores existan.
// El contrato será: verificar CanConsume → decrementar atómicamente (Lua/DB tx).
func (s *DefaultFeatureLimitService) Consume(
	ctx context.Context,
	userID uuid.UUID,
	featureKey string,
) error {
	// FUTURE: implementar cuando módulos consumidores existan.
	return domain.ErrNotImplemented
}
