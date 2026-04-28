package cache

// =============================================================================
// Cache de permisos de usuario con TTL y verificación rápida
// Evita consultas a DB para cada request
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// UserPermission representa los permisos cacheados de un usuario
type UserPermission struct {
	UserID      uuid.UUID `json:"user_id"`
	RoleID      uuid.UUID `json:"role_id"`
	RoleName    string    `json:"role_name"`
	Permissions []string  `json:"permissions"`
}

// PermissionCache cachea permisos de usuario con fallback a DB
type PermissionCache struct {
	dragonfly *Dragonfly
	prefix    string
	ttl       time.Duration
}

// NewPermissionCache crea un nuevo cache de permisos
func NewPermissionCache(df *Dragonfly, ttl time.Duration) *PermissionCache {
	if ttl <= 0 {
		ttl = 15 * time.Minute // Default: 15 minutos
	}
	return &PermissionCache{
		dragonfly: df,
		prefix:    HashtagPerm, // usa el hashtag correcto para evitar Global Lock
		ttl:       ttl,
	}
}

// Get obtiene permisos cacheados de un usuario
func (c *PermissionCache) Get(ctx context.Context, userID uuid.UUID) (*UserPermission, error) {
	key := PermissionKey(userID.String())
	data, err := c.dragonfly.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if data == "" {
		return nil, nil // Cache miss
	}

	var perm UserPermission
	if err := json.Unmarshal([]byte(data), &perm); err != nil {
		return nil, fmt.Errorf("unmarshaling permission: %w", err)
	}
	return &perm, nil
}

// Set cachea permisos de un usuario
func (c *PermissionCache) Set(ctx context.Context, perm *UserPermission) error {
	key := PermissionKey(perm.UserID.String())
	data, err := json.Marshal(perm)
	if err != nil {
		return fmt.Errorf("marshaling permission: %w", err)
	}
	return c.dragonfly.Set(ctx, key, data, c.ttl)
}

// Delete elimina permisos cacheados (ej. cuando se revoca un rol)
func (c *PermissionCache) Delete(ctx context.Context, userID uuid.UUID) error {
	key := PermissionKey(userID.String())
	return c.dragonfly.Delete(ctx, key)
}

// HasPermission verifica si el usuario tiene un permiso específico
// Devuelve (false, nil) si no hay cache o no tiene el permiso
func (c *PermissionCache) HasPermission(ctx context.Context, userID uuid.UUID, requiredPerm string) (bool, error) {
	perm, err := c.Get(ctx, userID)
	if err != nil {
		return false, err
	}
	if perm == nil {
		return false, nil
	}

	// Check específico o wildcard "*:*"
	for _, p := range perm.Permissions {
		if p == requiredPerm || p == "*:*" {
			return true, nil
		}
	}
	return false, nil
}

// Refresh actualiza el TTL de permisos cacheados
func (c *PermissionCache) Refresh(ctx context.Context, userID uuid.UUID) error {
	key := PermissionKey(userID.String())
	return c.dragonfly.Expire(ctx, key, c.ttl)
}
