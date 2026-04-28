// Repository: Interfaz para acceso a datos del perfil de usuario.
// Define las operaciones disponibles para el repositorio.
package domain

import (
	"context"

	"github.com/google/uuid"
)

// =============================================================================
// Repository - Interfaz para acceso a datos del perfil de usuario
// Alineada con migración user_profiles (fuente de truth)
// =============================================================================

type UserRepository interface {
	// UpsertProfile creates or updates a user profile
	// Uses user_id as conflict key (not id) - aligned with migration
	UpsertProfile(ctx context.Context, profile *UserProfile) error

	// GetByUserID retrieves a user profile by user_id (FK to Auth domain)
	GetByUserID(ctx context.Context, userID uuid.UUID) (*UserProfile, error)

	// GetByID retrieves a user profile by PK id
	GetByID(ctx context.Context, id uuid.UUID) (*UserProfile, error)

	// UpdateStatus updates the profile (for email verification)
	// Now uses user_id instead of id
	// TODO: Implement when user_profiles gets a status column.
	UpdateStatus(ctx context.Context, userID uuid.UUID, status UserProfileStatus) error

	// UpdateAvatar updates the profile avatar
	UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error

	// UpdatePreferences updates profile preferences
	UpdatePreferences(ctx context.Context, userID uuid.UUID, timezone, language, currency string, isPublic bool) error
}
