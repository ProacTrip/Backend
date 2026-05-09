// Caso de uso: Crear o actualizar perfil de usuario.
// Maneja la lógica de upsert para perfiles.
package upsert_profile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/features/shared"
)

// =============================================================================
// Upsert Profile Use Case - Patrón Upsert para perfiles de usuario
// Maneja la condición de carrera donde verify-email llega antes que profile creation
// Alineado con migración user_profiles
// =============================================================================

type UseCase struct {
	repo        domain.ProfileRepository
	travelRepo  domain.TravelPrefsRepository
	medicalRepo domain.MedicalProfileRepository
	notifRepo   domain.NotificationPrefsRepository
	rdb         *redis.Client // optional — for populating profile prefs cache
}

func NewUseCase(repo domain.ProfileRepository) *UseCase {
	return &UseCase{repo: repo}
}

// NewUseCaseWithCache creates a UseCase that also populates the Dragonfly
// profile prefs cache after profile creation/update.
func NewUseCaseWithCache(repo domain.ProfileRepository, rdb *redis.Client) *UseCase {
	return &UseCase{repo: repo, rdb: rdb}
}

// NewUseCaseComplete creates a UseCase that populates all related tables
// (travel prefs, medical profile, notification prefs) on profile creation.
// Repos may be nil for backward compat — defaults are skipped silently.
func NewUseCaseComplete(
	repo domain.ProfileRepository,
	travelRepo domain.TravelPrefsRepository,
	medicalRepo domain.MedicalProfileRepository,
	notifRepo domain.NotificationPrefsRepository,
	rdb *redis.Client,
) *UseCase {
	return &UseCase{
		repo:        repo,
		travelRepo:  travelRepo,
		medicalRepo: medicalRepo,
		notifRepo:   notifRepo,
		rdb:         rdb,
	}
}

// Execute creates or updates a user profile with optional environment-based prefs.
// El perfil se crea basado en user_id (FK al dominio Auth)
// El Upsert usa user_id como clave de conflicto
// email: denormalized from the registration event (avoids cross-DB joins).
func (uc *UseCase) Execute(ctx context.Context, userID uuid.UUID, email string, envPrefs ...domain.EnvPrefs) error {
	// Crear nuevo perfil con los defaults de la migración
	profile := domain.NewUserProfile(userID, email)

	// Override with environment prefs if provided
	if len(envPrefs) > 0 && envPrefs[0].HasAny() {
		prefs := envPrefs[0]
		profile.SetPreferences(
			new(prefs.TimezoneName),
			new(prefs.LanguageCode),
			new(prefs.CurrencyCode),
			false,
		)
	}

	// Upsert - creates if not exists, updates if exists
	// El ON CONFLICT usa user_id (no id)
	if err := uc.repo.UpsertProfile(ctx, profile); err != nil {
		return fmt.Errorf("upsert profile: %w", err)
	}

	// Create default related rows (idempotent — INSERT ON CONFLICT DO NOTHING)
	uc.createDefaults(ctx, userID)

	// Populate Dragonfly profile prefs cache (best-effort, non-blocking)
	uc.populatePrefsCache(ctx, userID, profile)

	return nil
}

// createDefaults seeds the related tables with default values.
// All inserts are idempotent (INSERT ... ON CONFLICT DO NOTHING in repos).
// Failures are logged as warnings — they don't block profile creation.
func (uc *UseCase) createDefaults(ctx context.Context, userID uuid.UUID) {
	if uc.travelRepo != nil {
		travelPrefs := domain.NewTravelPreferences(userID)
		if err := uc.travelRepo.Create(ctx, travelPrefs); err != nil {
			slog.WarnContext(ctx, "create travel prefs default failed",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()),
			)
		}
	}

	if uc.medicalRepo != nil {
		medicalProfile := domain.NewMedicalProfileV2(userID)
		if err := uc.medicalRepo.Create(ctx, medicalProfile); err != nil {
			slog.WarnContext(ctx, "create medical profile default failed",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()),
			)
		}
	}

	if uc.notifRepo != nil {
		np1 := domain.NewNotificationPreference(userID, domain.NotifChannelEmail, domain.NotifTypeBookingConfirm)
		if err := uc.notifRepo.Upsert(ctx, np1); err != nil {
			slog.WarnContext(ctx, "create notif pref default failed",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()),
			)
		}
		np2 := domain.NewNotificationPreference(userID, domain.NotifChannelEmail, domain.NotifTypeTravelReminder)
		if err := uc.notifRepo.Upsert(ctx, np2); err != nil {
			slog.WarnContext(ctx, "create notif pref default failed",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()),
			)
		}
	}
}

// populatePrefsCache stores profile preferences in Dragonfly for future search
// default resolution. This is fire-and-forget — failures are logged but never
// bubble up to the caller.
func (uc *UseCase) populatePrefsCache(ctx context.Context, userID uuid.UUID, profile *domain.UserProfile) {
	if uc.rdb == nil {
		return
	}

	// Use WithoutCancel so the cache write survives handler context cancellation
	bgCtx := context.WithoutCancel(ctx)

	if err := shared.SetProfilePrefs(bgCtx, uc.rdb,
		userID.String(),
		profile.CurrencyCode,
		profile.LanguageCode,
		"", // country_code not stored in profile
		profile.TimezoneName,
	); err != nil {
		slog.WarnContext(bgCtx, "populate profile prefs cache failed",
			slog.String("user_id", userID.String()),
			slog.String("error", err.Error()),
		)
	}
}

// HandleVerification handles the email verification event
// This is called when verify-email endpoint is hit
// Actualiza preferencias u otras configuraciones si es necesario
func (uc *UseCase) HandleVerification(ctx context.Context, userID uuid.UUID) error {
	// Check if profile exists by user_id
	existing, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil && !errors.Is(err, domain.ErrProfileNotFound) {
		return fmt.Errorf("check profile for verification: %w", err)
	}

	if existing == nil {
		// Edge case: verify-email clicked before any event was processed
		// Create a minimal profile first (email empty — will be filled by registration event)
		profile := domain.NewUserProfile(userID, "")
		if err := uc.repo.UpsertProfile(ctx, profile); err != nil {
			return fmt.Errorf("create profile on verification: %w", err)
		}
		uc.createDefaults(ctx, userID)
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
