// Repository: Interfaz para acceso a datos del perfil de usuario.
// Define las operaciones disponibles para el repositorio.
package domain

import (
	"context"

	"github.com/google/uuid"
)

// =============================================================================
// ProfileRepository — Perfil de usuario
// =============================================================================

type ProfileRepository interface {
	Create(ctx context.Context, profile *UserProfile) error
	UpsertProfile(ctx context.Context, profile *UserProfile) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*UserProfile, error)
	GetByID(ctx context.Context, id uuid.UUID) (*UserProfile, error)
	Update(ctx context.Context, profile *UserProfile) error
	UpdateLocale(ctx context.Context, userID uuid.UUID, language, currency string) error
	UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error
	UpdatePreferences(ctx context.Context, userID uuid.UUID, language, currency string) error
}

// =============================================================================
// TravelPrefsRepository — Preferencias de viaje
// =============================================================================

type TravelPrefsRepository interface {
	Create(ctx context.Context, prefs *TravelPreferences) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*TravelPreferences, error)
	Update(ctx context.Context, prefs *TravelPreferences) error
}

// =============================================================================
// MedicalProfileRepository — Perfil médico
// =============================================================================

type MedicalProfileRepository interface {
	Create(ctx context.Context, profile *MedicalProfile) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*MedicalProfile, error)
	Update(ctx context.Context, profile *MedicalProfile) error
}

// =============================================================================
// MedicalPendingUpdateRepository — Actualizaciones médicas pendientes
// =============================================================================

type MedicalPendingUpdateRepository interface {
	Create(ctx context.Context, update *MedicalPendingUpdate) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*MedicalPendingUpdate, error)
	GetByID(ctx context.Context, id uuid.UUID) (*MedicalPendingUpdate, error)
	Resolve(ctx context.Context, id uuid.UUID, status MedicalPendingUpdateStatus) error
	ListByUserID(ctx context.Context, userID uuid.UUID, status *MedicalPendingUpdateStatus) ([]*MedicalPendingUpdate, error)
	CountPending(ctx context.Context, userID uuid.UUID) (int, error)
}

// =============================================================================
// DocumentRepository — Documentos de usuario

type DocumentRepository interface {
	Create(ctx context.Context, doc *UserDocument) error
	GetByID(ctx context.Context, id uuid.UUID) (*UserDocument, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*UserDocument, error)
	CountByUserID(ctx context.Context, userID uuid.UUID) (int, error)
	Update(ctx context.Context, doc *UserDocument) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetTypes(ctx context.Context) ([]DocumentType, error)
}

// =============================================================================
// EncryptionService — Servicio de encriptación
// =============================================================================

// EncryptionService define las operaciones de encriptación/desencriptación
// requeridas para campos sensibles del módulo user.
type EncryptionService interface {
	Encrypt(plaintext string) ([]byte, error)
	Decrypt(ciphertext []byte) (string, error)
}
