// Servicio de dominio para resolución de permisos efectivos.
// Resuelve permisos solo desde el rol base — sin overrides.
package services

import (
	"context"
	"slices"

	"github.com/google/uuid"
)

// =============================================================================
// RolePermissionRepository — acceso a permisos del rol
// =============================================================================

// RolePermissionRepository obtiene los permisos base de un rol desde la DB.
// Implementación: adapters/postgres/dashboard_repository.go
type RolePermissionRepository interface {
	// GetPermissionsByRoleID retorna los códigos de permiso asignados a un rol.
	GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]string, error)
}

// =============================================================================
// PermissionResolver — interfaz del servicio
// =============================================================================

// PermissionResolver computa los permisos efectivos de un usuario desde su rol.
// Resolución simplificada: solo rol base, sin overrides.
type PermissionResolver interface {
	// ResolveEffectivePermissions retorna los permisos efectivos del usuario.
	// El userID se mantiene por compatibilidad de interfaz aunque solo
	// se use roleID para la resolución.
	ResolveEffectivePermissions(ctx context.Context, userID, roleID uuid.UUID) ([]string, error)
}

// =============================================================================
// DefaultPermissionResolver — implementación concreta
// =============================================================================

// DefaultPermissionResolver implementa PermissionResolver usando solo repos de rol.
type DefaultPermissionResolver struct {
	roleRepo RolePermissionRepository
}

// NewPermissionResolver crea un nuevo DefaultPermissionResolver.
func NewPermissionResolver(
	roleRepo RolePermissionRepository,
) *DefaultPermissionResolver {
	return &DefaultPermissionResolver{
		roleRepo: roleRepo,
	}
}

// ResolveEffectivePermissions retorna los permisos del rol, ordenados.
func (r *DefaultPermissionResolver) ResolveEffectivePermissions(
	ctx context.Context,
	userID, roleID uuid.UUID,
) ([]string, error) {
	// 1. Obtener permisos base del rol
	rolePerms, err := r.roleRepo.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	// 2. Ordenar para determinismo
	result := make([]string, len(rolePerms))
	copy(result, rolePerms)
	slices.Sort(result)

	return result, nil
}
