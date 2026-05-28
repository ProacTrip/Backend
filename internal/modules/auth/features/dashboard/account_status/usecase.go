// Lógica de negocio para habilitar/deshabilitar cuentas desde el dashboard.
// Orquesta validación, actualización DB, token_version++, invalidación simple de sesión cache,
// y publicación de eventos account_disabled/account_enabled via EventBus + SSE Hub.
package account_status

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/ProacTrip/Backend/internal/shared/session"
	sse "github.com/ProacTrip/Backend/internal/shared/sse"
)

// =============================================================================
// Puerto de repositorio — interfaz local que el adapter PG implementa
// =============================================================================

// AccountStatusRepo es el puerto local para operaciones de estado de cuenta.
// Implementado por el adapter postgres.
type AccountStatusRepo interface {
	// GetByID obtiene un usuario por su ID.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)

	// UpdateStatus actualiza el estado del usuario y retorna el nuevo token_version.
	// La query: UPDATE users SET status = $1, token_version = token_version + 1,
	// updated_at = NOW() WHERE id = $2 RETURNING token_version.
	// El incremento de token_version es ATÓMICO (UPDATE ... RETURNING).
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) (int, error)
}

// =============================================================================
// Puerto de EventPublisher — interfaz local para publicar eventos
// =============================================================================

// EventPublisher es el puerto local para publicar eventos de dominio.
type EventPublisher interface {
	Publish(ctx context.Context, stream string, payload map[string]interface{}) (string, error)
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orquesta el cambio de estado de cuenta con invalidación simple de
// sesión cache y publicación de eventos account_disabled/account_enabled.
// Single-session: invalida solo la key {auth}:session:{userID} (sin SCAN).
type UseCase struct {
	repo           AccountStatusRepo
	rdb            *redis.Client
	eventPublisher EventPublisher
}

// NewUseCase crea un nuevo use case de account status.
func NewUseCase(repo AccountStatusRepo, rdb *redis.Client, eventPublisher EventPublisher) *UseCase {
	return &UseCase{repo: repo, rdb: rdb, eventPublisher: eventPublisher}
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute ejecuta el cambio de estado.
// Flow: validate → get user → check self-disable → check no-op → DB update →
//
//	invalidate session cache (single-key delete) + publish account event.
//
// AS-SPEC-003: solo transiciones active↔disabled.
// AS-SPEC-005: token_version++ + sesión cache invalidada en disable/enable.
// El evento account_disabled/account_enabled se publica fire-and-forget con 2s timeout.
func (uc *UseCase) Execute(ctx context.Context, cmd EnableDisableCommand) (*Response, error) {
	// 1. Validar
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Obtener usuario actual
	user, err := uc.repo.GetByID(ctx, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user for status change: %w", err)
	}

	previousStatus := string(user.Status)

	// 3. No permitir deshabilitarse a sí mismo
	if cmd.UserID == cmd.ActorID && cmd.Status == "disabled" {
		return nil, domain.ErrCannotDisableSelf
	}

	// 4. No-op: el status ya es el mismo
	if previousStatus == cmd.Status {
		return nil, fmt.Errorf("%w: user already has status %q", domain.ErrInvalidInput, cmd.Status)
	}

	// 5. Actualizar en DB (token_version++ atómico dentro del UPDATE)
	newTV, err := uc.repo.UpdateStatus(ctx, cmd.UserID, cmd.Status)
	if err != nil {
		return nil, fmt.Errorf("update user status: %w", err)
	}

	// 6. Invalidar sesión cache (disable Y enable).
	// Single-session: solo borramos {auth}:session:{userID} (sin SCAN).
	// Best-effort: si falla, el TTL del cache (1 min) lo resuelve eventualmente.
	uc.invalidateSessionCache(ctx, cmd.UserID)

	// 7. Publicar evento account_disabled o account_enabled (fire-and-forget)
	//    El notification consumer lo consume para enviar el email correspondiente.
	if uc.eventPublisher != nil {
		if cmd.Status == "disabled" {
			uc.publishAccountDisabledEvent(cmd.UserID, user.Email, cmd.ActorID)
		} else if cmd.Status == "active" {
			uc.publishAccountEnabledEvent(cmd.UserID, user.Email, cmd.ActorID)
		}
	}

	return &Response{
		UserID:         cmd.UserID,
		PreviousStatus: previousStatus,
		NewStatus:      cmd.Status,
		TokenVersion:   newTV,
	}, nil
}

// =============================================================================
// Invalidación de sesión cache (single-key, best-effort)
// =============================================================================

// invalidateSessionCache elimina la entrada {auth}:session:{userID} del cache.
// Retorna sin error incluso si falla — es best-effort.
func (uc *UseCase) invalidateSessionCache(ctx context.Context, userID uuid.UUID) {
	if uc.rdb == nil {
		return
	}

	if err := session.InvalidateSession(ctx, uc.rdb, userID.String()); err != nil {
		// Log pero no fallar — token_version mismatch es la defensa primaria
		_ = err
	}
}

// =============================================================================
// Publicación de eventos account_disabled/account_enabled (fire-and-forget)
// =============================================================================

// publishAccountDisabledEvent publica el evento account_disabled en un goroutine
// con timeout de 2s. No bloquea la respuesta del handler.
// También notifica via SSE Hub para tiempo real.
func (uc *UseCase) publishAccountDisabledEvent(userID uuid.UUID, email string, actorID uuid.UUID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		now := time.Now().UnixMilli()
		flatPayload := map[string]any{
			"event_type":   "account_disabled",
			"aggregate_id": userID.String(),
			"timestamp":    now,
			"user_id":      userID.String(),
			"email":        email,
			"disabled_by":  actorID.String(),
		}
		stream := eventbus.StreamName("auth.account.events")
		_, err := uc.eventPublisher.Publish(ctx, stream, flatPayload)
		if err != nil {
			slog.ErrorContext(ctx, "failed to publish account_disabled event to Dragonfly",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()),
			)
		}

		// SSE Hub: fuerza logout en tiempo real al usuario deshabilitado.
		// PublishAndBridge: entrega local + cross-instance vía Dragonfly Pub/Sub.
		// Best-effort: si el Hub no está inicializado (ej. tests), se ignora silenciosamente.
		func() {
			defer func() { _ = recover() }()
			sse.GetHub().PublishAndBridge(ctx, uc.rdb, userID, sse.Event{
				Type: "account.disabled",
				Data: map[string]any{
					"reason":  "account_disabled",
					"user_id": userID.String(),
				},
			})
		}()
	}()
}

// publishAccountEnabledEvent publica el evento account_enabled en un goroutine
// con timeout de 2s. No bloquea la respuesta del handler.
// También notifica via SSE Hub para tiempo real.
func (uc *UseCase) publishAccountEnabledEvent(userID uuid.UUID, email string, actorID uuid.UUID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		now := time.Now().UnixMilli()
		flatPayload := map[string]any{
			"event_type":   "account_enabled",
			"aggregate_id": userID.String(),
			"timestamp":    now,
			"user_id":      userID.String(),
			"email":        email,
			"enabled_by":   actorID.String(),
		}
		stream := eventbus.StreamName("auth.account.events")
		_, err := uc.eventPublisher.Publish(ctx, stream, flatPayload)
		if err != nil {
			slog.ErrorContext(ctx, "failed to publish account_enabled event to Dragonfly",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()),
			)
		}

		// SSE Hub: notifica en tiempo real al usuario habilitado.
		// PublishAndBridge: entrega local + cross-instance vía Dragonfly Pub/Sub.
		// Best-effort: si el Hub no está inicializado (ej. tests), se ignora silenciosamente.
		func() {
			defer func() { _ = recover() }()
			sse.GetHub().PublishAndBridge(ctx, uc.rdb, userID, sse.Event{
				Type: "account.enabled",
				Data: map[string]any{
					"reason":  "account_enabled",
					"user_id": userID.String(),
				},
			})
		}()
	}()
}
