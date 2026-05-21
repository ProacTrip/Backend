package services

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// =============================================================================
// Mocks — implementaciones de repos para testing
// =============================================================================

type mockRoleRepo struct {
	perms []string
}

func (m *mockRoleRepo) GetPermissionsByRoleID(_ context.Context, _ uuid.UUID) ([]string, error) {
	return slices.Clone(m.perms), nil
}

// =============================================================================
// Tabla de tests — resolución solo-rol
// =============================================================================

func TestResolveEffectivePermissions(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name      string
		rolePerms []string
		want      []string
	}{
		{
			name:      "rol con permisos — retorna los permisos ordenados",
			rolePerms: []string{"users:write", "users:read"},
			want:      []string{"users:read", "users:write"},
		},
		{
			name:      "rol sin permisos — retorna slice vacío",
			rolePerms: nil,
			want:      nil,
		},
		{
			name:      "rol con un solo permiso",
			rolePerms: []string{"users:read"},
			want:      []string{"users:read"},
		},
		{
			name:      "admin con 5 permisos — ordenados",
			rolePerms: []string{"feature_limits:write", "sessions:write", "users:read", "sessions:read", "users:write"},
			want:      []string{"feature_limits:write", "sessions:read", "sessions:write", "users:read", "users:write"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			resolver := NewPermissionResolver(
				&mockRoleRepo{perms: tt.rolePerms},
			)

			got, err := resolver.ResolveEffectivePermissions(ctx, userID, roleID)
			if err != nil {
				t.Fatalf("ResolveEffectivePermissions: %v", err)
			}

			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
