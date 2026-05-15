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
// Implementa domain.ProfileRepository
// Alineado con migración user_profiles (fuente de truth)
// =============================================================================

// Compile-time interface checks
var (
	_ domain.ProfileRepository = (*UserRepository)(nil)
)

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
			user_id, email, first_name, last_name, phone, phone_verified,
			avatar_url, current_location, bio, timezone_name,
			language_code, currency_code, role, status, is_public, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (user_id) DO UPDATE SET
			email = COALESCE(EXCLUDED.email, user_profiles.email),
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
			role = EXCLUDED.role,
			status = EXCLUDED.status,
			is_public = EXCLUDED.is_public,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.Exec(ctx, query,
		profile.UserID,
		profile.Email,
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
		profile.Role,
		profile.Status,
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
		SELECT up.id, up.user_id, up.email,
		       up.first_name, up.last_name, up.date_of_birth, up.gender, up.nationality,
		       up.phone, up.phone_verified, up.avatar_url, up.current_location, up.bio,
		       up.timezone_name, up.language_code, up.currency_code, up.role, up.status, up.is_public,
		       up.created_at, up.updated_at
		FROM user_profiles up
		WHERE up.user_id = $1
	`

	var p domain.UserProfile
	var dob *time.Time
	var gender *domain.Gender

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&p.ID,
		&p.UserID,
		&p.Email,
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
		&p.Role,
		&p.Status,
		&p.IsPublic,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	p.DateOfBirth = dob
	p.Gender = gender

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get profile by user_id: %w", err)
	}

	return &p, nil
}

// GetByID recupera un perfil por PK id
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.UserProfile, error) {
	query := `
		SELECT up.id, up.user_id, up.email,
		       up.first_name, up.last_name, up.date_of_birth, up.gender, up.nationality,
		       up.phone, up.phone_verified, up.avatar_url, up.current_location, up.bio,
		       up.timezone_name, up.language_code, up.currency_code, up.role, up.status, up.is_public,
		       up.created_at, up.updated_at
		FROM user_profiles up
		WHERE up.id = $1
	`

	var p domain.UserProfile
	var dob *time.Time
	var gender *domain.Gender

	err := r.db.QueryRow(ctx, query, id).Scan(
		&p.ID,
		&p.UserID,
		&p.Email,
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
		&p.Role,
		&p.Status,
		&p.IsPublic,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	p.DateOfBirth = dob
	p.Gender = gender

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get profile by id: %w", err)
	}

	return &p, nil
}

// =============================================================================
// ProfileRepository — v2 methods
// =============================================================================

// Create inserta un nuevo perfil de usuario.
func (r *UserRepository) Create(ctx context.Context, profile *domain.UserProfile) error {
	return r.UpsertProfile(ctx, profile)
}

// Update realiza un update parcial del perfil usando COALESCE.
// Solo los campos no-nil en el struct se actualizan en la DB.
func (r *UserRepository) Update(ctx context.Context, profile *domain.UserProfile) error {
	query := `
		UPDATE user_profiles SET
			first_name     = COALESCE($2, first_name),
			last_name      = COALESCE($3, last_name),
			date_of_birth  = COALESCE($4, date_of_birth),
			gender         = COALESCE($5, gender),
			nationality    = COALESCE($6, nationality),
			phone          = COALESCE($7, phone),
			bio            = COALESCE($8, bio),
			is_public      = COALESCE($9, is_public),
			updated_at     = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.Exec(ctx, query,
		profile.UserID,
		profile.FirstName,
		profile.LastName,
		profile.DateOfBirth,
		profile.Gender,
		profile.Nationality,
		profile.Phone,
		profile.Bio,
		profile.IsPublic,
	)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}
	return nil
}

// UpdateLocale actualiza timezone, language, currency y current_location.
func (r *UserRepository) UpdateLocale(ctx context.Context, userID uuid.UUID, timezone, language, currency, currentLocation string) error {
	query := `
		UPDATE user_profiles SET
			timezone_name    = COALESCE(NULLIF($2, ''), timezone_name),
			language_code    = COALESCE(NULLIF($3, ''), language_code),
			currency_code    = COALESCE(NULLIF($4, ''), currency_code),
			current_location = COALESCE(NULLIF($5, ''), current_location),
			updated_at       = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.Exec(ctx, query, userID, timezone, language, currency, currentLocation)
	if err != nil {
		return fmt.Errorf("update locale: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}
	return nil
}

// =============================================================================
// Actualizaciones (original UserRepository methods)
// =============================================================================

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
		return domain.ErrProfileNotFound
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
		return domain.ErrProfileNotFound
	}

	return nil
}
