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
	sharedUser "github.com/ProacTrip/Backend/internal/shared/user"
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
// (travel prefs, medical profile) on profile creation.
// Repos may be nil for backward compat — defaults are skipped silently.
func NewUseCaseComplete(
	repo domain.ProfileRepository,
	travelRepo domain.TravelPrefsRepository,
	medicalRepo domain.MedicalProfileRepository,
	rdb *redis.Client,
) *UseCase {
	return &UseCase{
		repo:        repo,
		travelRepo:  travelRepo,
		medicalRepo: medicalRepo,
		rdb:         rdb,
	}
}

// Execute crea o actualiza un perfil de usuario con preferencias de entorno opcionales.
// El perfil se crea basado en user_id (FK al dominio Auth)
// El Upsert usa user_id como clave de conflicto
// email: desnormalizado del evento de registro (evita joins cross-DB).
// firstName: opcional, viene del evento de registro. Si está vacío, se deja nil.
// avatarURL: opcional, viene del evento de registro (ej. Google OAuth picture).
func (uc *UseCase) Execute(ctx context.Context, userID uuid.UUID, email string, firstName string, avatarURL string, envPrefs ...domain.EnvPrefs) error {
	// Crear nuevo perfil con los defaults de la migración
	profile := domain.NewUserProfile(userID, email)

	// Setear nombre si se proveyó en el registro
	if firstName != "" {
		profile.SetName(&firstName, nil)
	}

	// Setear avatar si se proveyó (ej. Google OAuth)
	if avatarURL != "" {
		profile.SetAvatar(new(avatarURL))
	}

	// Override with environment prefs if provided (language and currency only)
	if len(envPrefs) > 0 && envPrefs[0].HasAny() {
		prefs := envPrefs[0]
		profile.SetPreferences(
			new(prefs.LanguageCode),
			new(prefs.CurrencyCode),
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

// createDefaults genera las tablas relacionadas con valores por defecto.
// Todos los inserts son idempotentes (INSERT ... ON CONFLICT DO NOTHING en los repos).
// Los fallos se loguean como warnings — no bloquean la creación del perfil.
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
		medicalProfile := domain.NewMedicalProfile(userID)
		if err := uc.medicalRepo.Create(ctx, medicalProfile); err != nil {
			slog.WarnContext(ctx, "create medical profile default failed",
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

	// Usar WithoutCancel para que la escritura de cache sobreviva la cancelación del contexto del handler
	bgCtx := context.WithoutCancel(ctx)

	if err := sharedUser.SetProfilePrefs(bgCtx, uc.rdb, userID.String(), &sharedUser.Prefs{
		Currency: profile.CurrencyCode,
		Language: profile.LanguageCode,
		// country_code and timezone not stored in profile — cache reads them from registration event
	}); err != nil {
		slog.WarnContext(bgCtx, "populate profile prefs cache failed",
			slog.String("user_id", userID.String()),
			slog.String("error", err.Error()),
		)
	}
}

// HandleVerification maneja el evento de verificación de email.
// Se llama cuando el endpoint verify-email es accedido.
// Actualiza preferencias u otras configuraciones si es necesario.
func (uc *UseCase) HandleVerification(ctx context.Context, userID uuid.UUID) error {
	// Verificar si el perfil existe por user_id
	existing, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil && !errors.Is(err, domain.ErrProfileNotFound) {
		return fmt.Errorf("check profile for verification: %w", err)
	}

	if existing == nil {
		// Caso borde: verify-email cliqueado antes de que se procese cualquier evento
		// Crear un perfil mínimo primero (email vacío — será completado por el evento de registro)
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

// UpdatePreferences actualiza las preferencias del perfil (language y currency).
func (uc *UseCase) UpdatePreferences(ctx context.Context, userID uuid.UUID, language, currency string) error {
	return uc.repo.UpdatePreferences(ctx, userID, language, currency)
}
