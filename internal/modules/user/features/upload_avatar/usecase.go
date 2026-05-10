// Caso de uso: Generar URL prefirmada de R2 para subida de avatar.
package upload_avatar

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Ports
// =============================================================================

// StorageService define el puerto para operaciones de almacenamiento R2.
type StorageService interface {
	GenerateUploadURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	Storage StorageService
}

// UseCase implementa la generación de URL prefirmada para subir avatar.
type UseCase struct {
	storage StorageService
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{storage: deps.Storage}
}

// Execute genera una URL prefirmada de R2 para subir el avatar.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// Determinar TTL
	ttlMinutes := DefaultTTLMinutes
	if cmd.TTLMinutes != nil && *cmd.TTLMinutes > 0 {
		ttlMinutes = *cmd.TTLMinutes
	}
	expiry := time.Duration(ttlMinutes) * time.Minute

	// Extraer extensión del nombre de archivo
	ext := filepath.Ext(cmd.FileName)
	if ext == "" {
		ext = ".bin"
	}

	// Generar storage key: avatars/{user_id}/{uuid}.{ext}
	avatarUUID := uuid.Must(uuid.NewV7())
	storageKey := fmt.Sprintf("avatars/%s/%s%s", userID.String(), avatarUUID.String(), ext)

	// Generar URL prefirmada de R2
	uploadURL, err := uc.storage.GenerateUploadURL(ctx, "proactrip-assets", storageKey, expiry)
	if err != nil {
		return nil, fmt.Errorf("generar URL prefirmada: %w", err)
	}

	expiresAt := time.Now().Add(expiry)

	return &Response{
		UploadURL:  uploadURL,
		StorageKey: storageKey,
		ExpiresAt:  expiresAt,
		Message:    "Subí el archivo binario a upload_url, luego llamá a /avatar/confirm.",
	}, nil
}
