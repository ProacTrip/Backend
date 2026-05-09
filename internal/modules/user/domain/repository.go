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
	UpdateLocale(ctx context.Context, userID uuid.UUID, timezone, language, currency, currentLocation string) error
	UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error
	UpdatePreferences(ctx context.Context, userID uuid.UUID, timezone, language, currency string, isPublic bool) error
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
	Create(ctx context.Context, profile *MedicalProfileV2) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*MedicalProfileV2, error)
	Update(ctx context.Context, profile *MedicalProfileV2) error
}

// =============================================================================
// MedicalPendingUpdateRepository — Actualizaciones médicas pendientes
// =============================================================================

type MedicalPendingUpdateRepository interface {
	Create(ctx context.Context, update *MedicalPendingUpdate) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*MedicalPendingUpdate, error)
	GetByID(ctx context.Context, id uuid.UUID) (*MedicalPendingUpdate, error)
	Resolve(ctx context.Context, id uuid.UUID, status MedicalPendingUpdateStatus) error
	CountPending(ctx context.Context, userID uuid.UUID) (int, error)
}

// =============================================================================
// NotificationPrefsRepository — Preferencias de notificación
// =============================================================================

type NotificationPrefsRepository interface {
	Create(ctx context.Context, pref *NotificationPreference) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*NotificationPreference, error)
	Upsert(ctx context.Context, pref *NotificationPreference) error
	Delete(ctx context.Context, userID uuid.UUID, channel NotificationChannel, notifType NotificationType) error
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
// SavedSearchRepository — Búsquedas guardadas
// =============================================================================

type SavedSearchRepository interface {
	Create(ctx context.Context, search *SavedSearch) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*SavedSearch, error)
	GetByHash(ctx context.Context, userID uuid.UUID, searchHash string) (*SavedSearch, error)
	GetByID(ctx context.Context, id uuid.UUID) (*SavedSearch, error)
	Update(ctx context.Context, search *SavedSearch) error
	Delete(ctx context.Context, id uuid.UUID) error
	SetAlertEnabled(ctx context.Context, id uuid.UUID, enabled bool) error
}

// =============================================================================
// FavoriteRepository — Favoritos de usuario
// =============================================================================

type FavoriteRepository interface {
	Create(ctx context.Context, fav *Favorite) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Favorite, error)
	Delete(ctx context.Context, id uuid.UUID) error
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
