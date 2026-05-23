// RED phase — tests for account_status usecase.
// These reference types and functions that do NOT exist yet.
// They MUST fail to compile initially.
package account_status_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	accountstatus "github.com/ProacTrip/Backend/internal/modules/auth/features/dashboard/account_status"
)

// =============================================================================
// Stubs
// =============================================================================

type stubAccountStatusRepo struct {
	getByID      func(ctx context.Context, id uuid.UUID) (*domain.User, error)
	updateStatus func(ctx context.Context, id uuid.UUID, status string) (int, error)
}

func (s *stubAccountStatusRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.getByID(ctx, id)
}

func (s *stubAccountStatusRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (int, error) {
	return s.updateStatus(ctx, id, status)
}

// =============================================================================
// Fixtures
// =============================================================================

func usuarioActivo(id uuid.UUID, email string, roleID uuid.UUID) *domain.User {
	now := time.Now()
	return &domain.User{
		ID:            id,
		Email:         email,
		EmailVerified: true,
		RoleID:        roleID,
		RoleName:      "client",
		Status:        domain.StatusActive,
		TokenVersion:  1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func usuarioDeshabilitado(id uuid.UUID, email string, roleID uuid.UUID) *domain.User {
	u := usuarioActivo(id, email, roleID)
	u.Status = domain.StatusDisabled
	u.TokenVersion = 2
	return u
}

func newUseCase(
	getByID func(ctx context.Context, id uuid.UUID) (*domain.User, error),
	updateStatus func(ctx context.Context, id uuid.UUID, status string) (int, error),
	rdb *redis.Client,
) *accountstatus.UseCase {
	repo := &stubAccountStatusRepo{getByID: getByID, updateStatus: updateStatus}
	return accountstatus.NewUseCase(repo, rdb, nil) // nil eventPublisher para tests
}

func newMiniRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis.Start: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

// =============================================================================
// Tests — RED: code does NOT exist yet
// =============================================================================

// TestExecute_Disable_Success deshabilita usuario activo, incrementa token_version,
// invalida sesiones, y retorna respuesta con previous_status="active", new_status="disabled".
func TestExecute_Disable_Success(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "test@proactrip.com", roleID), nil
		},
		func(ctx context.Context, id uuid.UUID, status string) (int, error) {
			return 2, nil // nuevo token_version después del update
		},
		rdb,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "disabled",
		ActorID: actorID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if resp.UserID != userID {
		t.Errorf("resp.UserID = %s, expected %s", resp.UserID, userID)
	}
	if resp.PreviousStatus != "active" {
		t.Errorf("resp.PreviousStatus = %s, expected active", resp.PreviousStatus)
	}
	if resp.NewStatus != "disabled" {
		t.Errorf("resp.NewStatus = %s, expected disabled", resp.NewStatus)
	}
	if resp.TokenVersion != 2 {
		t.Errorf("resp.TokenVersion = %d, expected 2", resp.TokenVersion)
	}
	// AS-SPEC-005: sessions debe ser > 0 (si hay entradas cacheadas)
	// Con miniredis limpio, no hay sesiones que invalidar → count puede ser 0
}

// TestExecute_Enable_Success reactiva un usuario deshabilitado.
func TestExecute_Enable_Success(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioDeshabilitado(userID, "disabled@proactrip.com", roleID), nil
		},
		func(ctx context.Context, id uuid.UUID, status string) (int, error) {
			return 2, nil // token_version no cambia en enable según specs
		},
		rdb,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "active",
		ActorID: actorID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if resp.NewStatus != "active" {
		t.Errorf("resp.NewStatus = %s, expected active", resp.NewStatus)
	}
	if resp.PreviousStatus != "disabled" {
		t.Errorf("resp.PreviousStatus = %s, expected disabled", resp.PreviousStatus)
	}
}

// TestExecute_SelfDisable_Blocked bloquea el intento de deshabilitarse a sí mismo.
func TestExecute_SelfDisable_Blocked(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	// actorID == userID → self-disable
	actorID := userID

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "self@proactrip.com", roleID), nil
		},
		nil,
		rdb,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "disabled",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for self-disable, got nil")
	}

	// Debe ser domain.ErrCannotDisableSelf
	if !errors.Is(err, domain.ErrCannotDisableSelf) {
		t.Errorf("expected ErrCannotDisableSelf, got %v", err)
	}
}

// TestExecute_InvalidStatus_Suspended rechaza estado "suspended" (no válido para este endpoint).
func TestExecute_InvalidStatus_Suspended(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "test@proactrip.com", roleID), nil
		},
		nil,
		rdb,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "suspended",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for invalid status 'suspended', got nil")
	}
}

// TestExecute_InvalidStatus_PendingVerification rechaza estado no permitido.
func TestExecute_InvalidStatus_PendingVerification(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "test@proactrip.com", roleID), nil
		},
		nil,
		rdb,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "pending_verification",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for invalid status 'pending_verification', got nil")
	}
}

// TestExecute_UserNotFound retorna ErrUserNotFound del repo.
func TestExecute_UserNotFound(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	actorID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		nil,
		rdb,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  uuid.Must(uuid.NewV7()),
		Status:  "disabled",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// TestExecute_RepoUpdateError propaga errores del repositorio.
func TestExecute_RepoUpdateError(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())
	repoErr := errors.New("db connection lost")

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "test@proactrip.com", roleID), nil
		},
		func(ctx context.Context, id uuid.UUID, status string) (int, error) {
			return 0, repoErr
		},
		rdb,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "disabled",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error from repo update, got nil")
	}
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error to be wrapped, got %v", err)
	}
}

// TestValidate_EmptyUserID rechaza uuid.Nil.
func TestValidate_EmptyUserID(t *testing.T) {
	cmd := accountstatus.EnableDisableCommand{
		UserID:  uuid.Nil,
		Status:  "disabled",
		ActorID: uuid.Must(uuid.NewV7()),
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for nil UserID, got nil")
	}
}

// TestValidate_EmptyStatus rechaza status vacío.
func TestValidate_EmptyStatus(t *testing.T) {
	cmd := accountstatus.EnableDisableCommand{
		UserID:  uuid.Must(uuid.NewV7()),
		Status:  "",
		ActorID: uuid.Must(uuid.NewV7()),
	}
	err := cmd.Validate()
	if err == nil {
		t.Fatal("expected error for empty status, got nil")
	}
}

// TestValidate_StatusAlreadySame rechaza status igual al actual (no-op).
// Este test depende del flujo del usecase, no solo de Validate().
// Se verifica en TestExecute_NoopStatusChange.
func TestExecute_NoopStatusChange(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "active@proactrip.com", roleID), nil
		},
		nil, // No debería llamarse
		rdb,
	)

	// Intentar setear status "active" cuando ya está active
	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "active",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for same-status no-op, got nil")
	}
}

// TestExecute_SessionInvalidation_BestEffort verifica que la invalidación
// de sesión es best-effort: si Redis falla, la operación igual retorna éxito.
func TestExecute_SessionInvalidation_BestEffort(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	uc := newUseCase(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "test@proactrip.com", roleID), nil
		},
		func(ctx context.Context, id uuid.UUID, status string) (int, error) {
			return 2, nil
		},
		rdb,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "disabled",
		ActorID: actorID,
	}

	resp, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() should succeed even if session invalidation fails: %v", err)
	}
	if resp.NewStatus != "disabled" {
		t.Errorf("expected disabled, got %s", resp.NewStatus)
	}
	// Single-session: no hay SessionsInvalidated en la respuesta
}

// =============================================================================
// Stub EventPublisher para verificar publicación de eventos
// =============================================================================

// stubEventPublisher registra las llamadas a Publish para verificar en tests.
type stubEventPublisher struct {
	calls       []publishCall
	shouldError error // Si no es nil, Publish retorna este error.
}

type publishCall struct {
	stream  string
	payload map[string]interface{}
}

func (s *stubEventPublisher) Publish(_ context.Context, stream string, payload map[string]interface{}) (string, error) {
	s.calls = append(s.calls, publishCall{stream: stream, payload: payload})
	if s.shouldError != nil {
		return "", s.shouldError
	}
	return "msg-stub-001", nil
}

// newUseCaseWithPublisher crea un UseCase con EventPublisher inyectado.
func newUseCaseWithPublisher(
	getByID func(ctx context.Context, id uuid.UUID) (*domain.User, error),
	updateStatus func(ctx context.Context, id uuid.UUID, status string) (int, error),
	rdb *redis.Client,
	publisher accountstatus.EventPublisher,
) *accountstatus.UseCase {
	repo := &stubAccountStatusRepo{getByID: getByID, updateStatus: updateStatus}
	return accountstatus.NewUseCase(repo, rdb, publisher)
}

// =============================================================================
// Tests para publicación de eventos account_disabled/account_enabled (Task 2.1)
// =============================================================================

// TestExecute_Disable_PublishesAccountDisabledEvent verifica que al deshabilitar
// se publica el evento account_disabled en el stream correcto via goroutine.
func TestExecute_Disable_PublishesAccountDisabledEvent(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())
	email := "disabled-event@proactrip.com"

	publisher := &stubEventPublisher{}
	uc := newUseCaseWithPublisher(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, email, roleID), nil
		},
		func(ctx context.Context, id uuid.UUID, status string) (int, error) {
			return 2, nil
		},
		rdb,
		publisher,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "disabled",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	// Esperar un poco para que el goroutine de fire-and-forget termine.
	time.Sleep(100 * time.Millisecond)

	if len(publisher.calls) < 1 {
		t.Fatalf("expected at least 1 publish call (account_disabled), got %d", len(publisher.calls))
	}

	// Buscar account_disabled event
	var accountDisabledCall *publishCall
	for i := range publisher.calls {
		eventType, _ := publisher.calls[i].payload["event_type"].(string)
		if eventType == "account_disabled" {
			accountDisabledCall = &publisher.calls[i]
			break
		}
	}

	if accountDisabledCall == nil {
		t.Fatal("account_disabled event was not published")
	}

	payload := accountDisabledCall.payload
	if payload["user_id"] != userID.String() {
		t.Errorf("user_id = %q, want %q", payload["user_id"], userID.String())
	}
	if payload["email"] != email {
		t.Errorf("email = %q, want %q", payload["email"], email)
	}
	if payload["disabled_by"] != actorID.String() {
		t.Errorf("disabled_by = %q, want %q", payload["disabled_by"], actorID.String())
	}
}

// TestExecute_Enable_PublishesAccountEnabledEvent verifica que al habilitar
// se publica el evento account_enabled.
func TestExecute_Enable_PublishesAccountEnabledEvent(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())
	email := "enabled-event@proactrip.com"

	publisher := &stubEventPublisher{}
	uc := newUseCaseWithPublisher(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioDeshabilitado(userID, email, roleID), nil
		},
		func(ctx context.Context, id uuid.UUID, status string) (int, error) {
			return 3, nil
		},
		rdb,
		publisher,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "active",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	var accountEnabledCall *publishCall
	for i := range publisher.calls {
		eventType, _ := publisher.calls[i].payload["event_type"].(string)
		if eventType == "account_enabled" {
			accountEnabledCall = &publisher.calls[i]
			break
		}
	}

	if accountEnabledCall == nil {
		t.Fatal("account_enabled event was not published")
	}

	payload := accountEnabledCall.payload
	if payload["user_id"] != userID.String() {
		t.Errorf("user_id = %q, want %q", payload["user_id"], userID.String())
	}
	if payload["email"] != email {
		t.Errorf("email = %q, want %q", payload["email"], email)
	}
	if payload["enabled_by"] != actorID.String() {
		t.Errorf("enabled_by = %q, want %q", payload["enabled_by"], actorID.String())
	}
}

// TestExecute_SelfDisable_NoEventPublished verifica que NO se publica evento
// cuando el usuario intenta deshabilitarse a sí mismo (error temprano).
func TestExecute_SelfDisable_NoEventPublished(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())

	publisher := &stubEventPublisher{}
	uc := newUseCaseWithPublisher(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "self@proactrip.com", roleID), nil
		},
		nil,
		rdb,
		publisher,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "disabled",
		ActorID: userID, // self-disable
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for self-disable")
	}

	time.Sleep(50 * time.Millisecond)

	if len(publisher.calls) != 0 {
		t.Errorf("expected 0 events on self-disable error, got %d calls", len(publisher.calls))
	}
}

// TestExecute_Noop_NoEventPublished verifica que NO se publica evento
// cuando el status ya es el mismo (no-op).
func TestExecute_Noop_NoEventPublished(t *testing.T) {
	ctx := t.Context()
	rdb := newMiniRedis(t)

	userID := uuid.Must(uuid.NewV7())
	roleID := uuid.Must(uuid.NewV7())
	actorID := uuid.Must(uuid.NewV7())

	publisher := &stubEventPublisher{}
	uc := newUseCaseWithPublisher(
		func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return usuarioActivo(userID, "noop@proactrip.com", roleID), nil
		},
		nil,
		rdb,
		publisher,
	)

	cmd := accountstatus.EnableDisableCommand{
		UserID:  userID,
		Status:  "active",
		ActorID: actorID,
	}

	_, err := uc.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for no-op status change")
	}

	time.Sleep(50 * time.Millisecond)

	if len(publisher.calls) != 0 {
		t.Errorf("expected 0 events on no-op, got %d calls", len(publisher.calls))
	}
}
