// Caso de uso: Generar URL prefirmada de R2 para subida de avatar.
package upload_avatar

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/storage"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
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
	Storage     StorageService
	RateLimiter *ratelimit.RateLimiter // opcional
}

// UseCase implementa la generación de URL prefirmada para subir avatar.
type UseCase struct {
	storage     StorageService
	rateLimiter *ratelimit.RateLimiter
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		storage:     deps.Storage,
		rateLimiter: deps.RateLimiter,
	}
}

// CheckRateLimit verifica el rate limit para el usuario dado.
// Sigue el patrón de upload_document: cheapest check, bloquea spam antes de CPU/IO.
func (uc *UseCase) CheckRateLimit(ctx context.Context, userIDStr string) error {
	if uc.rateLimiter == nil {
		return nil
	}
	result, err := uc.rateLimiter.AuthenticatedAllow(ctx, userIDStr)
	if err != nil {
		return nil // degradar gracefully — permitir si falla la verificación
	}
	if !result.Allowed {
		return domain.ErrRateLimitExceeded
	}
	return nil
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
	uploadURL, err := uc.storage.GenerateUploadURL(ctx, storage.AssetsBucket(), storageKey, expiry)
	if err != nil {
		return nil, fmt.Errorf("generar URL prefirmada: %w", err)
	}

	expiresAt := time.Now().Add(expiry)

	return &Response{
		UploadURL:  uploadURL,
		StorageKey: storageKey,
		ExpiresAt:  expiresAt,
		EventsURL:  storage.SSEBaseURL(),
		Message:    "Subí el archivo binario a upload_url, luego llamá a /avatar/confirm.",
	}, nil
}
