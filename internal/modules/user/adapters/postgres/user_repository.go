// Adapter de PostgreSQL para perfiles de usuario.
// Implementa la interfaz del dominio.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// User Repository - PostgreSQL adapter
// Alineado con migración user_profiles (fuente de truth)
// =============================================================================

type UserRepository struct {
	db *pgxpool.Pool
}

// =============================================================================
// Constructor
// =============================================================================

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// =============================================================================
// Operaciones de perfil
// =============================================================================

// UpsertProfile implementa el patrón Upsert para perfiles de usuario.
// IMPORTANTE: Usa user_id como clave de conflicto (no id)
// La migración tiene: id (PK auto-generado) y user_id (UNIQUE, FK al auth)
func (r *UserRepository) UpsertProfile(ctx context.Context, profile *domain.UserProfile) error {
	// El conflicto debe ser en user_id (UNIQUE), no en id (PK)
	// Esto permite que el perfil se cree/actualice basado en el ID del usuario del dominio Auth
	query := `
		INSERT INTO user_profiles (
			user_id, first_name, last_name, phone, phone_verified,
			avatar_url, current_location, bio, timezone_name,
			language_code, currency_code, is_public, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (user_id) DO UPDATE SET
			first_name = COALESCE(EXCLUDED.first_name, user_profiles.first_name),
			last_name = COALESCE(EXCLUDED.last_name, user_profiles.last_name),
			phone = COALESCE(EXCLUDED.phone, user_profiles.phone),
			phone_verified = EXCLUDED.phone_verified,
			avatar_url = COALESCE(EXCLUDED.avatar_url, user_profiles.avatar_url),
			current_location = COALESCE(EXCLUDED.current_location, user_profiles.current_location),
			bio = COALESCE(EXCLUDED.bio, user_profiles.bio),
			timezone_name = COALESCE(EXCLUDED.timezone_name, user_profiles.timezone_name),
			language_code = COALESCE(EXCLUDED.language_code, user_profiles.language_code),
			currency_code = COALESCE(EXCLUDED.currency_code, user_profiles.currency_code),
			is_public = EXCLUDED.is_public,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.Exec(ctx, query,
		profile.UserID,
		profile.FirstName,
		profile.LastName,
		profile.Phone,
		profile.PhoneVerified,
		profile.AvatarURL,
		profile.CurrentLocation,
		profile.Bio,
		profile.TimezoneName,
		profile.LanguageCode,
		profile.CurrencyCode,
		profile.IsPublic,
		profile.CreatedAt,
		profile.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("upsert profile: %w", err)
	}

	return nil
}

// GetByUserID recupera un perfil por user_id
func (r *UserRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	query := `
		SELECT id, user_id, first_name, last_name, date_of_birth, gender, nationality,
		       phone, phone_verified, avatar_url, current_location, bio,
		       timezone_name, language_code, currency_code, is_public,
		       created_at, updated_at
		FROM user_profiles
		WHERE user_id = $1
	`

	var p domain.UserProfile
	var dob *time.Time
	var gender *domain.Gender

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&p.ID,
		&p.UserID,
		&p.FirstName,
		&p.LastName,
		&dob,
		&gender,
		&p.Nationality,
		&p.Phone,
		&p.PhoneVerified,
		&p.AvatarURL,
		&p.CurrentLocation,
		&p.Bio,
		&p.TimezoneName,
		&p.LanguageCode,
		&p.CurrencyCode,
		&p.IsPublic,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	p.DateOfBirth = dob
	p.Gender = gender

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get profile by user_id: %w", err)
	}

	return &p, nil
}

// GetByID recupera un perfil por PK id
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
	query := `
		SELECT id, user_id, first_name, last_name, date_of_birth, gender, nationality,
		       phone, phone_verified, avatar_url, current_location, bio,
		       timezone_name, language_code, currency_code, is_public,
		       created_at, updated_at
		FROM user_profiles
		WHERE id = $1
	`

	var p domain.UserProfile
	var dob *time.Time
	var gender *domain.Gender

	err := r.db.QueryRow(ctx, query, id).Scan(
		&p.ID,
		&p.UserID,
		&p.FirstName,
		&p.LastName,
		&dob,
		&gender,
		&p.Nationality,
		&p.Phone,
		&p.PhoneVerified,
		&p.AvatarURL,
		&p.CurrentLocation,
		&p.Bio,
		&p.TimezoneName,
		&p.LanguageCode,
		&p.CurrencyCode,
		&p.IsPublic,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	p.DateOfBirth = dob
	p.Gender = gender

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get profile by id: %w", err)
	}

	return &p, nil
}

// =============================================================================
// Actualizaciones
// =============================================================================

// UpdateStatus actualiza el estado del perfil
// Ahora usa user_id en vez de id
// TODO: Implement when user_profiles gets a status column.
// Currently dead code — user_profiles has no 'status' column
// and no callers exist in the codebase.
func (r *UserRepository) UpdateStatus(ctx context.Context, userID uuid.UUID, status domain.UserProfileStatus) error {
	query := `
		UPDATE user_profiles
		SET updated_at = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("update profile status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return errors.New("profile not found")
	}

	return nil
}

// UpdateAvatar actualiza el avatar
func (r *UserRepository) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	query := `
		UPDATE user_profiles
		SET avatar_url = $2, updated_at = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.Exec(ctx, query, userID, avatarURL)
	if err != nil {
		return fmt.Errorf("update avatar: %w", err)
	}

	if result.RowsAffected() == 0 {
		return errors.New("profile not found")
	}

	return nil
}

// UpdatePreferences actualiza las preferencias del perfil
func (r *UserRepository) UpdatePreferences(ctx context.Context, userID uuid.UUID, timezone, language, currency string, isPublic bool) error {
	query := `
		UPDATE user_profiles
		SET timezone_name = COALESCE(NULLIF($2, ''), timezone_name),
		    language_code = COALESCE(NULLIF($3, ''), language_code),
		    currency_code = COALESCE(NULLIF($4, ''), currency_code),
		    is_public = $5,
		    updated_at = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.Exec(ctx, query, userID, timezone, language, currency, isPublic)
	if err != nil {
		return fmt.Errorf("update preferences: %w", err)
	}

	if result.RowsAffected() == 0 {
		return errors.New("profile not found")
	}

	return nil
}
