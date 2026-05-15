// Servicio de dominio para resolución de permisos efectivos.
// Pipeline: (rol_permissions ∪ active_grants) − active_denies.
// Los overrides expirados se filtran antes de la unión/resta.
// Deny SIEMPRE gana (se aplica al final).
package services

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// RolePermissionRepository — acceso a permisos del rol
// =============================================================================

// RolePermissionRepository obtiene los permisos base de un rol desde la DB.
// Implementación: adapters/postgres/ (futuro — aún no creado).
type RolePermissionRepository interface {
	// GetPermissionsByRoleID retorna los códigos de permiso asignados a un rol.
	GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]string, error)
}

// =============================================================================
// PermissionOverrideRepository — acceso a overrides de permisos
// =============================================================================

// PermissionOverrideRepository obtiene los overrides de permisos de un usuario.
// Implementación: adapters/postgres/ (futuro — aún no creado).
type PermissionOverrideRepository interface {
	// GetOverridesByUserID retorna todos los overrides (grant y deny) para un usuario.
	// Incluye overrides expirados — el resolver los filtra.
	GetOverridesByUserID(ctx context.Context, userID uuid.UUID) ([]PermissionOverride, error)
}

// =============================================================================
// PermissionOverride — modelo de dominio para un override
// =============================================================================

// PermissionOverride representa un grant o deny de permiso para un usuario específico.
type PermissionOverride struct {
	UserID       uuid.UUID
	PermissionID uuid.UUID
	Permission   string // Código del permiso (ej. "users:write")
	Granted      bool   // true = grant, false = deny
	ExpiresAt    *time.Time
}

// =============================================================================
// PermissionResolver — interfaz del servicio
// =============================================================================

// PermissionResolver computa los permisos efectivos de un usuario aplicando
// el pipeline de resolución: rol base → overrides (grant/deny) → filtro expiry.
type PermissionResolver interface {
	// ResolveEffectivePermissions retorna los permisos efectivos del usuario.
	// Pipeline: (role_permissions ∪ active_grants) − active_denies.
	// Los overrides con expires_at < now se excluyen. Deny siempre gana.
	ResolveEffectivePermissions(ctx context.Context, userID, roleID uuid.UUID) ([]string, error)
}

// =============================================================================
// DefaultPermissionResolver — implementación concreta
// =============================================================================

// DefaultPermissionResolver implementa PermissionResolver usando repos de DB.
// Sin cacheo — el cache de sesión se maneja en la capa de middleware.
type DefaultPermissionResolver struct {
	roleRepo     RolePermissionRepository
	overrideRepo PermissionOverrideRepository
}

// NewPermissionResolver crea un nuevo DefaultPermissionResolver.
func NewPermissionResolver(
	roleRepo RolePermissionRepository,
	overrideRepo PermissionOverrideRepository,
) *DefaultPermissionResolver {
	return &DefaultPermissionResolver{
		roleRepo:     roleRepo,
		overrideRepo: overrideRepo,
	}
}

// ResolveEffectivePermissions implementa el pipeline de resolución.
func (r *DefaultPermissionResolver) ResolveEffectivePermissions(
	ctx context.Context,
	userID, roleID uuid.UUID,
) ([]string, error) {
	// 1. Obtener permisos base del rol
	rolePerms, err := r.roleRepo.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	// 2. Obtener overrides del usuario
	overrides, err := r.overrideRepo.GetOverridesByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 3. Separar grants y denies, filtrando expirados
	now := time.Now()
	var grants []string
	var denies []string

	for _, ov := range overrides {
		// Filtrar overrides expirados
		if ov.ExpiresAt != nil && now.After(*ov.ExpiresAt) {
			continue
		}

		if ov.Granted {
			grants = append(grants, ov.Permission)
		} else {
			denies = append(denies, ov.Permission)
		}
	}

	// 4. Unión: role_permissions ∪ active_grants
	effective := make(map[string]struct{})
	for _, p := range rolePerms {
		effective[p] = struct{}{}
	}
	for _, p := range grants {
		effective[p] = struct{}{}
	}

	// 5. Resta: remove active_denies (deny siempre gana)
	for _, p := range denies {
		delete(effective, p)
	}

	// 6. Convertir a slice ordenado (determinístico)
	result := make([]string, 0, len(effective))
	for p := range effective {
		result = append(result, p)
	}
	slices.Sort(result)

	return result, nil
}
