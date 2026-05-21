// Caso de uso: Confirmar subida de avatar y disparar validación asíncrona.
package confirm_avatar_upload

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/storage"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// Ports
// =============================================================================

// StorageService define el puerto para operaciones de almacenamiento R2.
type StorageService interface {
	Exists(ctx context.Context, bucket, key string) (bool, error)
}

// EventPublisher define el puerto para publicar eventos en Dragonfly Streams.
type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCaseDeps contiene las dependencias del caso de uso.
type UseCaseDeps struct {
	Storage        StorageService
	EventPublisher EventPublisher
}

// UseCase implementa la confirmación de subida de avatar.
type UseCase struct {
	storage        StorageService
	eventPublisher EventPublisher
}

// NewUseCase crea una nueva instancia del caso de uso.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		storage:        deps.Storage,
		eventPublisher: deps.EventPublisher,
	}
}

// Execute verifica que el archivo existe en R2 y dispara la validación asíncrona.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	// 1. Verificar que el archivo existe en R2
	exists, err := uc.storage.Exists(ctx, storage.AssetsBucket(), cmd.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("verificar existencia en R2: %w", err)
	}
	if !exists {
		return nil, domain.ErrAvatarNotFound
	}

	// 2. Generar avatar_id (UUID v7)
	avatarID := uuid.Must(uuid.NewV7())

	// 3. Publicar evento en Dragonfly Stream {events}:avatar:validate
	stream := fmt.Sprintf("{events}:avatar:validate")
	payload := map[string]interface{}{
		"user_id":     userID.String(),
		"storage_key": cmd.StorageKey,
		"avatar_id":   avatarID.String(),
		"timestamp":   fmt.Sprintf("%d", time.Now().UnixMilli()),
	}

	if _, err := uc.eventPublisher.Publish(ctx, stream, payload); err != nil {
		return nil, fmt.Errorf("publicar evento de validación: %w", err)
	}

	return &Response{
		Status:  "validating",
		Message: "Carga de avatar confirmada. Validación en progreso.",
	}, nil
}
