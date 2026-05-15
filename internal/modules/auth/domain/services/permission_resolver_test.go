package services

import (
	"context"
	"slices"
	"testing"
	"time"

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

type mockOverrideRepo struct {
	overrides []PermissionOverride
}

func (m *mockOverrideRepo) GetOverridesByUserID(_ context.Context, _ uuid.UUID) ([]PermissionOverride, error) {
	result := make([]PermissionOverride, len(m.overrides))
	copy(result, m.overrides)
	return result, nil
}

// =============================================================================
// Helpers
// =============================================================================

func ptrTime(t time.Time) *time.Time { return &t }

// =============================================================================
// Tabla de tests — pipeline de resolución
// =============================================================================

func TestResolveEffectivePermissions(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	tests := []struct {
		name      string
		rolePerms []string
		overrides []PermissionOverride
		want      []string
	}{
		{
			name:      "solo role permissions — sin overrides",
			rolePerms: []string{"users:read", "users:write"},
			overrides: nil,
			want:      []string{"users:read", "users:write"},
		},
		{
			name:      "role vacío — sin permisos base",
			rolePerms: nil,
			overrides: nil,
			want:      nil,
		},
		{
			name:      "grant agrega permiso más allá del rol",
			rolePerms: []string{"users:read"},
			overrides: []PermissionOverride{
				{UserID: userID, Permission: "users:write", Granted: true},
			},
			want: []string{"users:read", "users:write"},
		},
		{
			name:      "deny remueve permiso del rol",
			rolePerms: []string{"users:read", "users:write"},
			overrides: []PermissionOverride{
				{UserID: userID, Permission: "users:write", Granted: false},
			},
			want: []string{"users:read"},
		},
		{
			name:      "deny gana sobre grant — deny applied last",
			rolePerms: []string{"users:read"},
			overrides: []PermissionOverride{
				{UserID: userID, Permission: "users:write", Granted: true},
				{UserID: userID, Permission: "users:write", Granted: false},
			},
			want: []string{"users:read"},
		},
		{
			name:      "override expirado se ignora",
			rolePerms: []string{"users:read", "users:write"},
			overrides: []PermissionOverride{
				{UserID: userID, Permission: "users:write", Granted: false, ExpiresAt: ptrTime(past)},
			},
			want: []string{"users:read", "users:write"},
		},
		{
			name:      "override con expires_at futuro se aplica",
			rolePerms: []string{"users:read", "users:write"},
			overrides: []PermissionOverride{
				{UserID: userID, Permission: "users:write", Granted: false, ExpiresAt: ptrTime(future)},
			},
			want: []string{"users:read"},
		},
		{
			name:      "deny + grant combinados — deny gana",
			rolePerms: []string{"users:read", "roles:read"},
			overrides: []PermissionOverride{
				{UserID: userID, Permission: "users:write", Granted: true},
				{UserID: userID, Permission: "roles:read", Granted: false},
			},
			want: []string{"users:read", "users:write"},
		},
		{
			name:      "grant existente en rol no duplica",
			rolePerms: []string{"users:read"},
			overrides: []PermissionOverride{
				{UserID: userID, Permission: "users:read", Granted: true},
			},
			want: []string{"users:read"},
		},
		{
			name:      "deny sobre permiso que no tiene el rol — no-op",
			rolePerms: []string{"users:read"},
			overrides: []PermissionOverride{
				{UserID: userID, Permission: "roles:write", Granted: false},
			},
			want: []string{"users:read"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			resolver := NewPermissionResolver(
				&mockRoleRepo{perms: tt.rolePerms},
				&mockOverrideRepo{overrides: tt.overrides},
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
