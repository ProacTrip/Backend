// Adapter PostgreSQL para perfiles médicos v2.
// Implementa domain.MedicalProfileRepository.
// Usa JSONB para datos flexibles con encriptación por campo.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// MedicalProfileRepository — PostgreSQL adapter
// Alineado con migración user_medical_profiles
// =============================================================================

// Compile-time interface check
var _ domain.MedicalProfileRepository = (*MedicalProfileRepository)(nil)

// MedicalProfileRepository implementa domain.MedicalProfileRepository usando PostgreSQL.
type MedicalProfileRepository struct {
	db *pgxpool.Pool
}

// NewMedicalProfileRepository crea una nueva instancia del repositorio.
func NewMedicalProfileRepository(db *pgxpool.Pool) *MedicalProfileRepository {
	return &MedicalProfileRepository{db: db}
}

// =============================================================================
// Create — Inserta un perfil médico con JSONB '{}' vacío
// =============================================================================

// Create inserta un nuevo perfil médico para el usuario.
// El campo data se serializa a JSONB. Si Data es nil, se usa '{}'.
func (r *MedicalProfileRepository) Create(ctx context.Context, profile *domain.MedicalProfile) error {
	dataJSON, err := marshalMedicalData(profile.Data)
	if err != nil {
		return fmt.Errorf("create medical profile: marshal data: %w", err)
	}

	query := `
		INSERT INTO user_medical_profiles (
			id, user_id, data, is_shared, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO NOTHING
	`

	_, err = r.db.Exec(ctx, query,
		profile.ID,
		profile.UserID,
		dataJSON,
		profile.IsShared,
		profile.CreatedAt,
		profile.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create medical profile: %w", err)
	}

	return nil
}

// =============================================================================
// GetByUserID — Recupera perfil médico por user_id
// =============================================================================

// GetByUserID recupera el perfil médico del usuario.
// Retorna ErrMedicalProfileNotFound si no existe.
func (r *MedicalProfileRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.MedicalProfile, error) {
	query := `
		SELECT id, user_id, data, is_shared, created_at, updated_at
		FROM user_medical_profiles
		WHERE user_id = $1
	`

	var mp domain.MedicalProfile
	var dataBytes []byte

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&mp.ID,
		&mp.UserID,
		&dataBytes,
		&mp.IsShared,
		&mp.CreatedAt,
		&mp.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrMedicalProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get medical profile by user_id: %w", err)
	}

	// Deserializar JSONB → map[string]*MedicalFieldValue
	mp.Data, err = unmarshalMedicalData(dataBytes)
	if err != nil {
		return nil, fmt.Errorf("get medical profile: unmarshal data: %w", err)
	}

	return &mp, nil
}

// =============================================================================
// Update — Actualiza el perfil médico (JSONB data + is_shared)
// =============================================================================

// Update persiste los cambios del perfil médico.
// Serializa Data a JSONB y actualiza is_shared y updated_at.
func (r *MedicalProfileRepository) Update(ctx context.Context, profile *domain.MedicalProfile) error {
	dataJSON, err := marshalMedicalData(profile.Data)
	if err != nil {
		return fmt.Errorf("update medical profile: marshal data: %w", err)
	}

	query := `
		UPDATE user_medical_profiles
		SET data = $2, is_shared = $3, updated_at = NOW()
		WHERE user_id = $1
	`

	result, err := r.db.Exec(ctx, query,
		profile.UserID,
		dataJSON,
		profile.IsShared,
	)
	if err != nil {
		return fmt.Errorf("update medical profile: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrMedicalProfileNotFound
	}

	return nil
}

// =============================================================================
// Helpers — serialización JSONB
// =============================================================================

// marshalMedicalData serializa el mapa de datos médicos a JSONB ([]byte).
func marshalMedicalData(data map[string]*domain.MedicalFieldValue) ([]byte, error) {
	if data == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal medical data: %w", err)
	}
	return b, nil
}

// unmarshalMedicalData deserializa JSONB ([]byte) al mapa de datos médicos.
func unmarshalMedicalData(data []byte) (map[string]*domain.MedicalFieldValue, error) {
	if len(data) == 0 {
		return make(map[string]*domain.MedicalFieldValue), nil
	}
	result := make(map[string]*domain.MedicalFieldValue)
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal medical data: %w", err)
	}
	return result, nil
}
