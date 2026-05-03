package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// OAuthRepository implementa domain.OAuthRepository usando pgx.
// Compile-time check para asegurar que satisface la interfaz.
var _ domain.OAuthRepository = (*OAuthRepository)(nil)

type OAuthRepository struct {
	pool PgxPool
}

// NewOAuthRepository crea un nuevo repositorio OAuth.
func NewOAuthRepository(pool PgxPool) *OAuthRepository {
	return &OAuthRepository{pool: pool}
}

// CreateAuthIdentity inserta una nueva identidad de autenticación externa.
func (r *OAuthRepository) CreateAuthIdentity(ctx context.Context, identity *domain.AuthIdentity) error {
	query := `
		INSERT INTO user_auth_identities (
			id, user_id, provider_code, provider_user_id, email,
			display_name, avatar_url, access_token_enc, refresh_token_enc,
			token_expires_at, raw_data, last_used_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := r.pool.Exec(ctx, query,
		identity.ID,
		identity.UserID,
		identity.ProviderCode,
		identity.ProviderUserID,
		identity.Email,
		identity.DisplayName,
		identity.AvatarURL,
		identity.AccessTokenEnc,
		identity.RefreshTokenEnc,
		identity.TokenExpiresAt,
		identity.RawData,
		identity.LastUsedAt,
		identity.CreatedAt,
		identity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert auth identity: %w", err)
	}
	return nil
}

// GetAuthIdentityByProvider obtiene una identidad por proveedor y provider_user_id.
func (r *OAuthRepository) GetAuthIdentityByProvider(ctx context.Context, providerCode, providerUserID string) (*domain.AuthIdentity, error) {
	query := `
		SELECT id, user_id, provider_code, provider_user_id, email,
		       display_name, avatar_url, access_token_enc, refresh_token_enc,
		       token_expires_at, raw_data, last_used_at, created_at, updated_at
		FROM user_auth_identities
		WHERE provider_code = $1 AND provider_user_id = $2
	`

	var identity domain.AuthIdentity
	err := r.pool.QueryRow(ctx, query, providerCode, providerUserID).Scan(
		&identity.ID,
		&identity.UserID,
		&identity.ProviderCode,
		&identity.ProviderUserID,
		&identity.Email,
		&identity.DisplayName,
		&identity.AvatarURL,
		&identity.AccessTokenEnc,
		&identity.RefreshTokenEnc,
		&identity.TokenExpiresAt,
		&identity.RawData,
		&identity.LastUsedAt,
		&identity.CreatedAt,
		&identity.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrIdentityNotFound
		}
		return nil, fmt.Errorf("get auth identity by provider: %w", err)
	}
	return &identity, nil
}

// GetAuthIdentitiesByUser obtiene todas las identidades vinculadas a un usuario.
func (r *OAuthRepository) GetAuthIdentitiesByUser(ctx context.Context, userID uuid.UUID) ([]*domain.AuthIdentity, error) {
	query := `
		SELECT id, user_id, provider_code, provider_user_id, email,
		       display_name, avatar_url, access_token_enc, refresh_token_enc,
		       token_expires_at, raw_data, last_used_at, created_at, updated_at
		FROM user_auth_identities
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get auth identities by user: %w", err)
	}
	defer rows.Close()

	var identities []*domain.AuthIdentity
	for rows.Next() {
		var identity domain.AuthIdentity
		err := rows.Scan(
			&identity.ID,
			&identity.UserID,
			&identity.ProviderCode,
			&identity.ProviderUserID,
			&identity.Email,
			&identity.DisplayName,
			&identity.AvatarURL,
			&identity.AccessTokenEnc,
			&identity.RefreshTokenEnc,
			&identity.TokenExpiresAt,
			&identity.RawData,
			&identity.LastUsedAt,
			&identity.CreatedAt,
			&identity.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan auth identity: %w", err)
		}
		identities = append(identities, &identity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate auth identities: %w", err)
	}

	if len(identities) == 0 {
		return nil, domain.ErrIdentityNotFound
	}

	return identities, nil
}

// UpdateAuthIdentity actualiza una identidad de autenticación existente.
func (r *OAuthRepository) UpdateAuthIdentity(ctx context.Context, identity *domain.AuthIdentity) error {
	query := `
		UPDATE user_auth_identities SET
			email = $1, display_name = $2, avatar_url = $3,
			access_token_enc = $4, refresh_token_enc = $5,
			token_expires_at = $6, raw_data = $7, last_used_at = $8,
			updated_at = $9
		WHERE id = $10
	`

	ct, err := r.pool.Exec(ctx, query,
		identity.Email,
		identity.DisplayName,
		identity.AvatarURL,
		identity.AccessTokenEnc,
		identity.RefreshTokenEnc,
		identity.TokenExpiresAt,
		identity.RawData,
		identity.LastUsedAt,
		identity.UpdatedAt,
		identity.ID,
	)
	if err != nil {
		return fmt.Errorf("update auth identity: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("update auth identity: %w", domain.ErrIdentityNotFound)
	}

	return nil
}
