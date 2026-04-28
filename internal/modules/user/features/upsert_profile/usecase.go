// Caso de uso: Crear o actualizar perfil de usuario.
// Maneja la lógica de upsert para perfiles.
package upsert_profile

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Upsert Profile Use Case - Patrón Upsert para perfiles de usuario
// Maneja la condición de carrera donde verify-email llega antes que profile creation
// Alineado con migración user_profiles
// =============================================================================

type UseCase struct {
	repo domain.UserRepository
}

func NewUseCase(repo domain.UserRepository) *UseCase {
	return &UseCase{repo: repo}
}

// Execute creates or updates a user profile.
// El perfil se crea basado en user_id (FK al dominio Auth)
// El Upsert usa user_id como clave de conflicto
func (uc *UseCase) Execute(ctx context.Context, userID uuid.UUID) error {
	// Crear nuevo perfil con los defaults de la migración
	profile := domain.NewUserProfile(userID)

	// Upsert - creates if not exists, updates if exists
	// El ON CONFLICT usa user_id (no id)
	if err := uc.repo.UpsertProfile(ctx, profile); err != nil {
		return fmt.Errorf("upsert profile: %w", err)
	}

	return nil
}

// HandleVerification handles the email verification event
// This is called when verify-email endpoint is hit
// Actualiza preferencias u otras configuraciones si es necesario
func (uc *UseCase) HandleVerification(ctx context.Context, userID uuid.UUID) error {
	// Check if profile exists by user_id
	existing, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("check profile for verification: %w", err)
	}

	if existing == nil {
		// Edge case: verify-email clicked before any event was processed
		// Create a minimal profile first
		profile := domain.NewUserProfile(userID)
		if err := uc.repo.UpsertProfile(ctx, profile); err != nil {
			return fmt.Errorf("create profile on verification: %w", err)
		}
	}

	// El status del perfil ahora se maneja de forma diferente
	// No hay campo status en la migración de user_profiles directamente

	return nil
}

// SetAvatar establece el avatar del perfil
func (uc *UseCase) SetAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	return uc.repo.UpdateAvatar(ctx, userID, avatarURL)
}

// UpdatePreferences actualiza las preferencias del perfil
func (uc *UseCase) UpdatePreferences(ctx context.Context, userID uuid.UUID, timezone, language, currency string, isPublic bool) error {
	return uc.repo.UpdatePreferences(ctx, userID, timezone, language, currency, isPublic)
}
