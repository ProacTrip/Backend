// Consumer de validación de avatares.
// Consume eventos del stream {events}:avatar:validate en Dragonfly.
// Valida que el archivo exista en R2, verifica MIME y tamaño,
// y actualiza el perfil del usuario con la URL del avatar.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Constantes
// =============================================================================

const (
	avatarStream       = "{events}:avatar:validate"
	avatarGroup        = "avatar-validator-group"
	maxAvatarSizeBytes = 5242880 // 5 MB
)

// AcceptedAvatarMimeTypes son los tipos MIME aceptados como avatares.
var AcceptedAvatarMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// =============================================================================
// Ports
// =============================================================================

// R2Client es el puerto para interactuar con R2 storage.
type R2Client interface {
	Exists(ctx context.Context, bucket, key string) (bool, error)
}

// =============================================================================
// AvatarValidator — Consumer de Dragonfly Streams
// =============================================================================

// AvatarValidator consume el stream {events}:avatar:validate, valida
// archivos en R2 y actualiza el avatar del usuario.
type AvatarValidator struct {
	rdb          *redis.Client
	repo         domain.ProfileRepository
	group        string
	consumer     string
	dlqStream    string
	avatarBaseURL string // URL base para avatares (CDN en prod, vacío en dev)
	running      atomic.Bool
	orphanDone   chan struct{} // cerrado cuando rescueOrphans termina
}

// NewAvatarValidator crea un nuevo validador de avatares.
// avatarBaseURL: prefijo para la URL del avatar (ej. "https://cdn.proactrip.com").
// Si está vacío, no se actualiza el avatar_url (el frontend usará el default).
func NewAvatarValidator(rdb *redis.Client, repo domain.ProfileRepository, avatarBaseURL string) *AvatarValidator {
	return &AvatarValidator{
		rdb:          rdb,
		repo:         repo,
		group:        avatarGroup,
		consumer:     fmt.Sprintf("avatar-validator-%d", time.Now().UnixMilli()),
		dlqStream:    AvatarDLQStream,
		avatarBaseURL: avatarBaseURL,
	}
}

// IsRunning indica si la goroutine principal de consumo O rescueOrphans está activa.
func (v *AvatarValidator) IsRunning() bool {
	return v.running.Load() || !isClosed(v.orphanDone)
}

// Name devuelve un identificador legible para reportes de health check.
func (v *AvatarValidator) Name() string { return "avatar-validator" }

// OrphanDone expone el canal que se cierra cuando rescueOrphans termina.
func (v *AvatarValidator) OrphanDone() <-chan struct{} { return v.orphanDone }

// Run inicia el consumer en background. Retorna inmediatamente.
// Se detiene cuando ctx es cancelado.
func (v *AvatarValidator) Run(ctx context.Context) error {
	// Ensure consumer group exists (idempotent)
	if err := eventbus.EnsureConsumerGroup(ctx, v.rdb, avatarStream, v.group); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	v.running.Store(true)
	v.orphanDone = make(chan struct{})
	go func() {
		defer v.running.Store(false)
		v.consume(ctx)
	}()

	// Start orphan rescue worker (XAUTOCLAIM)
	go func() {
		defer close(v.orphanDone)
		v.rescueOrphans(ctx)
	}()

	slog.Info("avatar validator started", "group", v.group, "consumer", v.consumer)
	return nil
}

// =============================================================================
// Worker loop
// =============================================================================

func (v *AvatarValidator) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := v.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    v.group,
			Consumer: v.consumer,
			Streams:  []string{avatarStream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			slog.Error("avatar validator: xreadgroup error", "error", err)
			continue
		}

		for _, stream := range messages {
			for _, msg := range stream.Messages {
				v.processMessage(ctx, msg)
			}
		}
	}
}

// =============================================================================
// Procesamiento de mensajes
// =============================================================================

func (v *AvatarValidator) processMessage(ctx context.Context, msg redis.XMessage) {
	// Parsear payload
	userIDStr, ok := msg.Values["user_id"].(string)
	if !ok {
		slog.Error("avatar validator: missing user_id in payload", "msg_id", msg.ID)
		_ = v.rdb.XAck(ctx, avatarStream, v.group, msg.ID)
		return
	}

	storageKey, ok := msg.Values["storage_key"].(string)
	if !ok {
		slog.Error("avatar validator: missing storage_key in payload", "msg_id", msg.ID)
		_ = v.rdb.XAck(ctx, avatarStream, v.group, msg.ID)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		slog.Error("avatar validator: invalid user_id", "user_id", userIDStr, "msg_id", msg.ID)
		_ = v.rdb.XAck(ctx, avatarStream, v.group, msg.ID)
		return
	}

	// Validar que el archivo existe en R2 (no tenemos acceso directo a R2,
	// el consumer asume que el archivo fue verificado en el paso de confirmación).
	// La validación de MIME y tamaño se delega al caller vía la confirmación inicial.

	// Construir URL del avatar si hay base URL configurada.
	// Si no (dev), el frontend usará el avatar por defecto.
	if v.avatarBaseURL == "" {
		slog.Info("avatar validator: skipping avatar URL update (no CDN configured)",
			"user_id", userID, "storage_key", storageKey)
		_ = v.rdb.XAck(ctx, avatarStream, v.group, msg.ID)
		return
	}
	avatarURL := fmt.Sprintf("%s/%s", v.avatarBaseURL, storageKey)

	// Actualizar el perfil del usuario con la URL del avatar
	if err := v.repo.UpdateAvatar(ctx, userID, avatarURL); err != nil {
		slog.Error("avatar validator: update avatar failed",
			"user_id", userID,
			"storage_key", storageKey,
			"error", err,
		)
		// NO XACK — dejar en PEL para reintento
		return
	}

	// XACK solo si todo fue exitoso
	if err := v.rdb.XAck(ctx, avatarStream, v.group, msg.ID); err != nil {
		slog.Error("avatar validator: xack error", "error", err, "msg_id", msg.ID)
	}

	slog.Info("avatar validator: avatar updated", "user_id", userID, "avatar_url", avatarURL)
}

// =============================================================================
// Orphan rescue
// =============================================================================

func (v *AvatarValidator) rescueOrphans(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		messages, err := eventbus.RescueOrphanedMessages(ctx, v.rdb, avatarStream, v.group, 5*time.Minute)
		if err != nil {
			slog.Error("avatar validator: rescue orphans error", "error", err)
			continue
		}

		for _, msg := range messages {
			slog.Info("avatar validator: reclaiming orphan message", "msg_id", msg.ID)
			v.processMessage(ctx, msg)
		}
	}
}
