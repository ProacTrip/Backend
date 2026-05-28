// Adapter de PostgreSQL para perfiles de usuario.
// Implementa la interfaz del dominio.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
// Usa user_id como clave de conflicto primaria. Si hay conflicto de email
// (uq_profiles_email) porque otro user_id ya tiene ese email, se reasigna
// el perfil al nuevo user_id. Esto cubre re-registros OAuth con el mismo email.
func (r *UserRepository) UpsertProfile(ctx context.Context, profile *domain.UserProfile) error {
	// Try the standard upsert first (ON CONFLICT user_id)
	query := `
		INSERT INTO user_profiles (
			user_id, email, first_name, last_name, phone,
			avatar_url, bio,
			language_code, currency_code, role, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (user_id) DO UPDATE SET
			email = COALESCE(EXCLUDED.email, user_profiles.email),
			first_name = COALESCE(EXCLUDED.first_name, user_profiles.first_name),
			last_name = COALESCE(EXCLUDED.last_name, user_profiles.last_name),
			phone = COALESCE(EXCLUDED.phone, user_profiles.phone),
			avatar_url = COALESCE(EXCLUDED.avatar_url, user_profiles.avatar_url),
			bio = COALESCE(EXCLUDED.bio, user_profiles.bio),
			language_code = COALESCE(EXCLUDED.language_code, user_profiles.language_code),
			currency_code = COALESCE(EXCLUDED.currency_code, user_profiles.currency_code),
			role = COALESCE(EXCLUDED.role, user_profiles.role),
			status = COALESCE(EXCLUDED.status, user_profiles.status),
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.Exec(ctx, query,
		profile.UserID,
		profile.Email,
		profile.FirstName,
		profile.LastName,
		profile.Phone,
		profile.AvatarURL,
		profile.Bio,
		profile.LanguageCode,
		profile.CurrencyCode,
		profile.Role,
		profile.Status,
		profile.CreatedAt,
		profile.UpdatedAt,
	)

	if err != nil {
		// If the email already belongs to another user_id (uq_profiles_email),
		// reassign the profile to the new user_id. This handles OAuth re-registration
		// where the same email gets a new user_id.
		if isUniqueViolation(err, "uq_profiles_email") {
			reassignQuery := `
				INSERT INTO user_profiles (
					user_id, email, first_name, last_name, phone,
					avatar_url, bio,
					language_code, currency_code, role, status, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
				ON CONFLICT (email) DO UPDATE SET
					user_id = EXCLUDED.user_id,
					first_name = COALESCE(EXCLUDED.first_name, user_profiles.first_name),
					last_name = COALESCE(EXCLUDED.last_name, user_profiles.last_name),
					avatar_url = COALESCE(EXCLUDED.avatar_url, user_profiles.avatar_url),
					updated_at = EXCLUDED.updated_at
			`
			_, err = r.db.Exec(ctx, reassignQuery,
				profile.UserID, profile.Email, profile.FirstName,
				profile.LastName, profile.Phone, profile.AvatarURL,
				profile.Bio, profile.LanguageCode, profile.CurrencyCode,
				profile.Role, profile.Status, profile.CreatedAt, profile.UpdatedAt,
			)
		}
		if err != nil {
			return fmt.Errorf("upsert profile: %w", err)
		}
	}

	return nil
}

// isUniqueViolation checks if a PostgreSQL error is a unique constraint violation.
func isUniqueViolation(err error, constraintName string) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505" && strings.Contains(err.Error(), constraintName)
	}
	return false
}

// GetByUserID recupera un perfil por user_id
func (r *UserRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	query := `
		SELECT up.id, up.user_id, up.email,
		       up.first_name, up.last_name, up.date_of_birth, up.gender, up.nationality,
		       up.phone, up.avatar_url, up.bio,
		       up.language_code, up.currency_code, up.role, up.status,
		       up.ocr_populated,
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
		&p.AvatarURL,
		&p.Bio,
		&p.LanguageCode,
		&p.CurrencyCode,
		&p.Role,
		&p.Status,
		&p.OcrPopulated,
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
		       up.phone, up.avatar_url, up.bio,
		       up.language_code, up.currency_code, up.role, up.status,
		       up.ocr_populated,
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
		&p.AvatarURL,
		&p.Bio,
		&p.LanguageCode,
		&p.CurrencyCode,
		&p.Role,
		&p.Status,
		&p.OcrPopulated,
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

// Update realiza un update parcial del perfil usando NULLIF + COALESCE.
// Los campos no-nil se actualizan; los nil preservan el valor existente.
// Para strings, NULLIF('', ...) evita que strings vacías sobreescriban — se tratan como "no tocar".
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
			language_code  = COALESCE($9, language_code),
			currency_code  = COALESCE($10, currency_code),
			ocr_populated  = $11,
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
		profile.LanguageCode,
		profile.CurrencyCode,
		profile.OcrPopulated,
	)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}
	return nil
}

// UpdateLocale actualiza language y currency del perfil.
func (r *UserRepository) UpdateLocale(ctx context.Context, userID uuid.UUID, language, currency string) error {
	query := `
		UPDATE user_profiles SET
			language_code    = COALESCE($2, language_code),
			currency_code    = COALESCE($3, currency_code),
			updated_at       = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.Exec(ctx, query, userID, language, currency)
	if err != nil {
		return fmt.Errorf("update locale: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}
	return nil
}

// =============================================================================
// Actualizaciones
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

// UpdatePreferences actualiza language y currency del perfil.
func (r *UserRepository) UpdatePreferences(ctx context.Context, userID uuid.UUID, language, currency string) error {
	return r.UpdateLocale(ctx, userID, language, currency)
}
