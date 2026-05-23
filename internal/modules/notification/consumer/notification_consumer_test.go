// Tests para el consumer de notificaciones.
// Cubre: processMessage con payload válido/inválido, ackOrWarn sin crash,
// IsRunning, Name.
//
// Convenciones:
//   - White-box testing (package consumer).
//   - Table-driven con t.Run(), nombres de sub-tests en español.
//   - Solo stdlib testing, sin testify.
//   - miniredis para mockear Redis.
package consumer

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/config"
	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
	"github.com/ProacTrip/Backend/internal/modules/notification/features/send_account_status_email"
	"github.com/ProacTrip/Backend/internal/modules/notification/features/send_verification_email"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
)

// =============================================================================
// Test doubles para dependencias del usecase
// =============================================================================

// mockEmailSender implementa send_verification_email.EmailSender para tests.
// Registra las llamadas para verificar que el usecase despachó correctamente.
type mockEmailSender struct {
	called     bool
	to         string
	templateID string
	vars       map[string]any
}

func (m *mockEmailSender) SendWithTemplate(_ context.Context, to, templateID string, vars map[string]any) (string, error) {
	m.called = true
	m.to = to
	m.templateID = templateID
	m.vars = vars
	return "msg_resend_test_123", nil
}

// mockNotificationRepo implementa domain.NotificationRepository para tests.
// Save retorna uuid.Nil (notificación nueva, sin conflicto de idempotencia).
type mockNotificationRepo struct{}

func (m *mockNotificationRepo) Save(_ context.Context, _ *domain.Notification) (uuid.UUID, error) {
	return uuid.Nil, nil
}
func (m *mockNotificationRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Notification, error) {
	return nil, nil
}
func (m *mockNotificationRepo) MarkSent(_ context.Context, _ uuid.UUID) error { return nil }

// =============================================================================
// Helpers
// =============================================================================

// newTestConsumer crea un NotificationConsumer conectado a miniredis.
// El usecase es nil porque la mayoría de tests de processMessage no lo usan.
func newTestConsumer(t *testing.T) (*NotificationConsumer, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	_ = eventbus.EnsureConsumerGroup(t.Context(), client,
		eventbus.StreamName("auth.user.registered"),
		"notification-service",
	)

	nc := &NotificationConsumer{
		rdb:        client,
		usecase:    nil, // Nil es válido para tests de processMessage que no despachan user_registered.
		group:      "notification-service",
		consumer:   "test-worker",
		streamName: eventbus.StreamName("auth.user.registered"),
	}

	return nc, mr
}

// newTestConsumerWithUsecase crea un NotificationConsumer con un usecase real
// respaldado por mocks (mockEmailSender + mockNotificationRepo).
// Retorna el consumer, el mock sender (para verificar llamadas) y miniredis.
func newTestConsumerWithUsecase(t *testing.T) (*NotificationConsumer, *mockEmailSender, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	_ = eventbus.EnsureConsumerGroup(t.Context(), client,
		eventbus.StreamName("auth.user.registered"),
		"notification-service",
	)

	sender := &mockEmailSender{}
	repo := &mockNotificationRepo{}
	uc := send_verification_email.NewUseCase(send_verification_email.Deps{
		Repo:           repo,
		Sender:         sender,
		FrontendConfig: config.FrontendConfig{DevURL: "http://localhost:3000"},
	})

	nc := &NotificationConsumer{
		rdb:        client,
		usecase:    uc,
		group:      "notification-service",
		consumer:   "test-worker",
		streamName: eventbus.StreamName("auth.user.registered"),
	}

	return nc, sender, mr
}

// =============================================================================
// H6.1 — TestProcessMessage_SinEventType: mensaje sin event_type → ack y nil error
// =============================================================================

func TestProcessMessage_SinEventType(t *testing.T) {
	nc, _ := newTestConsumer(t)

	msg := redis.XMessage{
		ID: "msg-no-event-type",
		Values: map[string]interface{}{
			"user_id": "11111111-1111-1111-1111-111111111111",
		},
	}

	err := nc.processMessage(t.Context(), msg)
	if err != nil {
		t.Errorf("processMessage sin event_type debería retornar nil (ACK silencioso), obtuvo: %v", err)
	}
}

// =============================================================================
// H6.2 — TestProcessMessage_EventTypeInvalido: tipo no string → ack y nil error
// =============================================================================

func TestProcessMessage_EventTypeInvalido(t *testing.T) {
	nc, _ := newTestConsumer(t)

	msg := redis.XMessage{
		ID: "msg-invalid-type",
		Values: map[string]interface{}{
			"event_type": 42, // No es string — debería hacer ACK y seguir.
		},
	}

	err := nc.processMessage(t.Context(), msg)
	if err != nil {
		t.Errorf("processMessage con event_type no-string debería retornar nil, obtuvo: %v", err)
	}
}

// =============================================================================
// H6.3 — TestProcessMessage_EventTypeDesconocido: tipo no reconocido → ack + warn
// =============================================================================

func TestProcessMessage_EventTypeDesconocido(t *testing.T) {
	nc, _ := newTestConsumer(t)

	msg := redis.XMessage{
		ID: "msg-unknown-event",
		Values: map[string]interface{}{
			"event_type": "some_future_event",
		},
	}

	err := nc.processMessage(t.Context(), msg)
	if err != nil {
		t.Errorf("processMessage con event_type desconocido debería retornar nil (ACK), obtuvo: %v", err)
	}
}

// =============================================================================
// H6.4 — TestAckOrWarn_NoCrash: ackOrWarn nunca debe crashear
// =============================================================================

func TestAckOrWarn_NoCrash(t *testing.T) {
	nc, _ := newTestConsumer(t)

	// Probar con un msg ID cualquiera — no existe en el stream,
	// debería loguear warning pero nunca crashear.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ackOrWarn hizo panic: %v", r)
		}
	}()

	nc.ackOrWarn(t.Context(), "msg-no-existe-123")
}

// =============================================================================
// H6.5 — TestHandleUserRegistered_SinEmail: payload sin email retorna error
// =============================================================================

func TestHandleUserRegistered_SinEmail(t *testing.T) {
	nc, _ := newTestConsumer(t)

	payload := map[string]interface{}{
		"user_id": "11111111-1111-1111-1111-111111111111",
		// email ausente
	}

	err := nc.handleUserRegistered(t.Context(), payload)
	if err == nil {
		t.Error("se esperaba error por email ausente")
	}
}

// =============================================================================
// H6.6 — TestHandleUserRegistered_SinUserID: payload sin user_id retorna error
// =============================================================================

func TestHandleUserRegistered_SinUserID(t *testing.T) {
	nc, _ := newTestConsumer(t)

	payload := map[string]interface{}{
		"email": "user@example.com",
		// user_id ausente
	}

	err := nc.handleUserRegistered(t.Context(), payload)
	if err == nil {
		t.Error("se esperaba error por user_id ausente")
	}
}

// =============================================================================
// H6.7 — TestHandleUserRegistered_UserIDInvalido: UUID mal formado retorna error
// =============================================================================

func TestHandleUserRegistered_UserIDInvalido(t *testing.T) {
	nc, _ := newTestConsumer(t)

	payload := map[string]interface{}{
		"user_id": "no-es-un-uuid",
		"email":   "user@example.com",
	}

	err := nc.handleUserRegistered(t.Context(), payload)
	if err == nil {
		t.Error("se esperaba error por UUID inválido")
	}
}

// =============================================================================
// H6.8 — TestIsRunning_EstadoInicial: recién creado, no está corriendo
// =============================================================================

func TestIsRunning_EstadoInicial(t *testing.T) {
	nc, _ := newTestConsumer(t)

	if nc.IsRunning() {
		t.Error("IsRunning debería ser false antes de Start()")
	}
}

// =============================================================================
// H6.9 — TestName: retorna identificador correcto
// =============================================================================

func TestName(t *testing.T) {
	nc, _ := newTestConsumer(t)

	name := nc.Name()
	if name != "notification-consumer" {
		t.Errorf("Name() = %q, se esperaba 'notification-consumer'", name)
	}
}

// =============================================================================
// H6.10 — TestNewNotificationConsumer_NoNil: constructor no retorna nil
// =============================================================================

func TestNewNotificationConsumer_NoNil(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	nc := NewNotificationConsumer(client, nil, nil)
	if nc == nil {
		t.Fatal("NewNotificationConsumer retornó nil")
	}
	if nc.Name() != "notification-consumer" {
		t.Errorf("Name() = %q, se esperaba 'notification-consumer'", nc.Name())
	}
}

// =============================================================================
// H6.11 — TestHandleUserRegistered_PayloadCompleto: payload completo despacha usecase
// =============================================================================

func TestHandleUserRegistered_PayloadCompleto(t *testing.T) {
	nc, sender, _ := newTestConsumerWithUsecase(t)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	payload := map[string]interface{}{
		"user_id":            userID.String(),
		"email":              "test@example.com",
		"verification_token": "tok_abc123",
		"first_name":         "Juan",
	}

	err := nc.handleUserRegistered(t.Context(), payload)
	if err != nil {
		t.Fatalf("handleUserRegistered con payload completo falló: %v", err)
	}

	if !sender.called {
		t.Fatal("se esperaba que el sender fuera invocado")
	}
	if sender.to != "test@example.com" {
		t.Errorf("email = %q, se esperaba %q", sender.to, "test@example.com")
	}
	if sender.templateID != send_verification_email.ResendTemplateVerifyEmail {
		t.Errorf("templateID = %q, se esperaba %q", sender.templateID, send_verification_email.ResendTemplateVerifyEmail)
	}
	if sender.vars["first_name"] != "Juan" {
		t.Errorf("first_name en vars = %v, se esperaba 'Juan'", sender.vars["first_name"])
	}
}

// =============================================================================
// H6.12 — TestProcessMessage_UsuarioRegistrado: dispatch completo con ACK
// =============================================================================

func TestProcessMessage_UsuarioRegistrado(t *testing.T) {
	nc, sender, _ := newTestConsumerWithUsecase(t)

	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	streamName := nc.streamName

	// XADD: publicar mensaje en el stream
	_, err := nc.rdb.XAdd(t.Context(), &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]interface{}{
			"event_type":         "user_registered",
			"user_id":            userID.String(),
			"email":              "test@example.com",
			"verification_token": "tok_abc123",
			"first_name":         "Juan",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	// XREADGROUP: reclamar mensaje (lo mueve a PEL)
	msgs, err := nc.rdb.XReadGroup(t.Context(), &redis.XReadGroupArgs{
		Group:    nc.group,
		Consumer: nc.consumer,
		Streams:  []string{streamName, ">"},
		Count:    1,
		Block:    0,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup falló: %v", err)
	}
	if len(msgs) == 0 || len(msgs[0].Messages) == 0 {
		t.Fatal("XReadGroup no retornó mensajes")
	}

	claimedMsg := msgs[0].Messages[0]

	// Verificar que el mensaje está en PEL antes de procesar
	pending, err := nc.rdb.XPending(t.Context(), streamName, nc.group).Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count == 0 {
		t.Fatal("se esperaba que el mensaje estuviera en PEL después de XREADGROUP")
	}

	// Procesar el mensaje
	err = nc.processMessage(t.Context(), claimedMsg)
	if err != nil {
		t.Fatalf("processMessage falló: %v", err)
	}

	// Verificar que el sender fue invocado
	if !sender.called {
		t.Fatal("se esperaba que el sender fuera invocado")
	}

	// Verificar que el mensaje fue ACK'd (fuera del PEL)
	pending, err = nc.rdb.XPending(t.Context(), streamName, nc.group).Result()
	if err != nil {
		t.Fatalf("XPending post-ACK falló: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("se esperaba PEL vacío después de ACK, count = %d", pending.Count)
	}
}

// =============================================================================
// H6.13 — TestAckOrWarn_MensajeValido: ACK exitoso de un mensaje en el PEL
// =============================================================================

func TestAckOrWarn_MensajeValido(t *testing.T) {
	nc, _ := newTestConsumer(t)

	streamName := nc.streamName

	// XADD: publicar mensaje
	msgID, err := nc.rdb.XAdd(t.Context(), &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]interface{}{"test": "data"},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	// XREADGROUP: reclamar mensaje para ponerlo en PEL
	_, err = nc.rdb.XReadGroup(t.Context(), &redis.XReadGroupArgs{
		Group:    nc.group,
		Consumer: nc.consumer,
		Streams:  []string{streamName, ">"},
		Count:    1,
		Block:    0,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup falló: %v", err)
	}

	// Ejecutar ACK
	nc.ackOrWarn(t.Context(), msgID)

	// Verificar que el mensaje ya no está en PEL
	pending, err := nc.rdb.XPending(t.Context(), streamName, nc.group).Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("se esperaba PEL vacío después de ACK, count = %d", pending.Count)
	}
}

// =============================================================================
// H6.14 — TestStart_Lifecycle: IsRunning refleja el ciclo de vida del consumer
// =============================================================================

func TestStart_Lifecycle(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	nc := &NotificationConsumer{
		rdb:        client,
		usecase:    nil,
		group:      "notification-service",
		consumer:   "test-worker-lifecycle",
		streamName: eventbus.StreamName("auth.user.registered"),
	}

	if nc.IsRunning() {
		t.Error("IsRunning debería ser false antes de Start()")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := nc.Start(ctx); err != nil {
		t.Fatalf("Start falló: %v", err)
	}

	// Dar tiempo al goroutine para arrancar
	time.Sleep(50 * time.Millisecond)

	if !nc.IsRunning() {
		t.Error("IsRunning debería ser true después de Start()")
	}

	// Cancelar el contexto para detener el consumer
	cancel()

	// Cerrar miniredis para desbloquear XReadGroup (que tiene Block=5s)
	mr.Close()
	// Pequeña pausa para que los goroutines detecten el error y ctx.Done()
	time.Sleep(50 * time.Millisecond)

	if nc.IsRunning() {
		t.Error("IsRunning debería ser false después de cancelar el contexto")
	}
}

// =============================================================================
// Account Status UseCase mock para tests de dispatch
// =============================================================================

// mockAccountStatusSender implementa send_account_status_email.EmailSender para tests.
type mockAccountStatusSender struct {
	called     bool
	to         string
	templateID string
	vars       map[string]any
	shouldErr  error
}

func (m *mockAccountStatusSender) SendWithTemplate(_ context.Context, to, templateID string, vars map[string]any) (string, error) {
	m.called = true
	m.to = to
	m.templateID = templateID
	m.vars = vars
	if m.shouldErr != nil {
		return "", m.shouldErr
	}
	return "msg_acc_status_stub_001", nil
}

// newTestConsumerWithAccountStatus crea un NotificationConsumer con ambos usecases
// respaldados por mocks (mockEmailSender para verification, mockAccountStatusSender para account status).
func newTestConsumerWithAccountStatus(t *testing.T) (*NotificationConsumer, *mockAccountStatusSender, *mockEmailSender, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	_ = eventbus.EnsureConsumerGroup(t.Context(), client,
		eventbus.StreamName("auth.user.registered"),
		"notification-service",
	)

	accSender := &mockAccountStatusSender{}
	verifSender := &mockEmailSender{}
	repo := &mockNotificationRepo{}

	accUC := send_account_status_email.NewUseCase(send_account_status_email.Deps{
		Repo:   repo,
		Sender: accSender,
	})
	verifUC := send_verification_email.NewUseCase(send_verification_email.Deps{
		Repo:           repo,
		Sender:         verifSender,
		FrontendConfig: config.FrontendConfig{DevURL: "http://localhost:3000"},
	})

	nc := &NotificationConsumer{
		rdb:             client,
		usecase:         verifUC,
		accountStatusUC: accUC,
		group:           "notification-service",
		consumer:        "test-worker-acc-status",
		streamName:      eventbus.StreamName("auth.user.registered"),
	}

	return nc, accSender, verifSender, mr
}

// =============================================================================
// H6.15 — TestHandleAccountDisabled_PayloadCompleto: dispatch con payload completo
// =============================================================================

func TestHandleAccountDisabled_PayloadCompleto(t *testing.T) {
	nc, accSender, _, _ := newTestConsumerWithAccountStatus(t)

	userID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	payload := map[string]any{
		"user_id":     userID.String(),
		"email":       "disabled-test@example.com",
		"disabled_by": "admin-001",
	}

	err := nc.handleAccountDisabled(t.Context(), payload)
	if err != nil {
		t.Fatalf("handleAccountDisabled con payload completo falló: %v", err)
	}

	if !accSender.called {
		t.Fatal("se esperaba que el sender fuera invocado")
	}
	if accSender.templateID != send_account_status_email.TemplateAccountDisabled {
		t.Errorf("TemplateID = %q, want %q", accSender.templateID, send_account_status_email.TemplateAccountDisabled)
	}
	if accSender.to != "disabled-test@example.com" {
		t.Errorf("to = %q, want %q", accSender.to, "disabled-test@example.com")
	}
	if accSender.vars["user_email"] != "disabled-test@example.com" {
		t.Errorf("user_email in vars = %v, want 'disabled-test@example.com'", accSender.vars["user_email"])
	}
}

// =============================================================================
// H6.16 — TestHandleAccountEnabled_PayloadCompleto: dispatch con payload completo
// =============================================================================

func TestHandleAccountEnabled_PayloadCompleto(t *testing.T) {
	nc, accSender, _, _ := newTestConsumerWithAccountStatus(t)

	userID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	payload := map[string]any{
		"user_id":    userID.String(),
		"email":      "enabled-test@example.com",
		"enabled_by": "admin-002",
	}

	err := nc.handleAccountEnabled(t.Context(), payload)
	if err != nil {
		t.Fatalf("handleAccountEnabled con payload completo falló: %v", err)
	}

	if !accSender.called {
		t.Fatal("se esperaba que el sender fuera invocado")
	}
	if accSender.templateID != send_account_status_email.TemplateAccountEnabled {
		t.Errorf("TemplateID = %q, want %q", accSender.templateID, send_account_status_email.TemplateAccountEnabled)
	}
	if accSender.to != "enabled-test@example.com" {
		t.Errorf("to = %q, want %q", accSender.to, "enabled-test@example.com")
	}
	if accSender.vars["user_email"] != "enabled-test@example.com" {
		t.Errorf("user_email in vars = %v, want 'enabled-test@example.com'", accSender.vars["user_email"])
	}
}

// =============================================================================
// H6.17 — TestHandleAccountDisabled_SinEmail: payload sin email → error (queda en PEL)
// =============================================================================

func TestHandleAccountDisabled_SinEmail(t *testing.T) {
	nc, _ := newTestConsumer(t)

	payload := map[string]any{
		"user_id": "22222222-2222-2222-2222-222222222222",
		// email ausente
	}

	err := nc.handleAccountDisabled(t.Context(), payload)
	if err == nil {
		t.Error("handleAccountDisabled sin email debería retornar error (missing email)")
	}
}

// H6.17b — TestHandleAccountEnabled_SinEmail: payload sin email → error (missing email)
func TestHandleAccountEnabled_SinEmail(t *testing.T) {
	nc, _ := newTestConsumer(t)

	payload := map[string]any{
		"user_id": "33333333-3333-3333-3333-333333333333",
		// email ausente
	}

	err := nc.handleAccountEnabled(t.Context(), payload)
	if err == nil {
		t.Error("handleAccountEnabled sin email debería retornar error (missing email)")
	}
}

// =============================================================================
// H6.18 — TestHandleAccountDisabled_UserIDInvalido: UUID mal formado → ACK + log
// =============================================================================

func TestHandleAccountDisabled_UserIDInvalido(t *testing.T) {
	nc, _ := newTestConsumer(t)

	payload := map[string]any{
		"user_id": "no-es-un-uuid",
		"email":   "invalid-uuid@example.com",
	}

	err := nc.handleAccountDisabled(t.Context(), payload)
	if err != nil {
		t.Errorf("handleAccountDisabled con UUID inválido debería retornar nil (ACK), obtuvo: %v", err)
	}
}

// =============================================================================
// H6.19 — TestHandleAccountDisabled_SinUserID: payload sin user_id → ACK
// =============================================================================

func TestHandleAccountDisabled_SinUserID(t *testing.T) {
	nc, _ := newTestConsumer(t)

	payload := map[string]any{
		"email": "nouserid@example.com",
	}

	err := nc.handleAccountDisabled(t.Context(), payload)
	if err != nil {
		t.Errorf("handleAccountDisabled sin user_id debería retornar nil (ACK), obtuvo: %v", err)
	}
}

// =============================================================================
// H6.21 — TestProcessMessage_AccountDisabled: dispatch completo con ACK (integración)
// =============================================================================

func TestProcessMessage_AccountDisabled(t *testing.T) {
	nc, accSender, _, _ := newTestConsumerWithAccountStatus(t)

	userID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	streamName := nc.streamName

	// XADD: publicar mensaje account_disabled
	_, err := nc.rdb.XAdd(t.Context(), &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{
			"event_type":  "account_disabled",
			"user_id":     userID.String(),
			"email":       "integration-disabled@example.com",
			"disabled_by": "admin-099",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	// XREADGROUP: reclamar mensaje
	msgs, err := nc.rdb.XReadGroup(t.Context(), &redis.XReadGroupArgs{
		Group:    nc.group,
		Consumer: nc.consumer,
		Streams:  []string{streamName, ">"},
		Count:    1,
		Block:    0,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup falló: %v", err)
	}
	if len(msgs) == 0 || len(msgs[0].Messages) == 0 {
		t.Fatal("XReadGroup no retornó mensajes")
	}

	claimedMsg := msgs[0].Messages[0]

	// Procesar el mensaje
	err = nc.processMessage(t.Context(), claimedMsg)
	if err != nil {
		t.Fatalf("processMessage falló: %v", err)
	}

	// Verificar que el sender fue invocado con el template correcto
	if !accSender.called {
		t.Fatal("se esperaba que el sender fuera invocado")
	}
	if accSender.templateID != send_account_status_email.TemplateAccountDisabled {
		t.Errorf("TemplateID = %q, want %q", accSender.templateID, send_account_status_email.TemplateAccountDisabled)
	}

	// Verificar que el mensaje fue ACK'd (fuera del PEL)
	pending, err := nc.rdb.XPending(t.Context(), streamName, nc.group).Result()
	if err != nil {
		t.Fatalf("XPending post-ACK falló: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("se esperaba PEL vacío después de ACK, count = %d", pending.Count)
	}
}

// =============================================================================
// H6.22 — TestProcessMessage_AccountEnabled: dispatch completo con ACK (integración)
// =============================================================================

func TestProcessMessage_AccountEnabled(t *testing.T) {
	nc, accSender, _, _ := newTestConsumerWithAccountStatus(t)

	userID := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	streamName := nc.streamName

	_, err := nc.rdb.XAdd(t.Context(), &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{
			"event_type": "account_enabled",
			"user_id":    userID.String(),
			"email":      "integration-enabled@example.com",
			"enabled_by": "admin-100",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	msgs, err := nc.rdb.XReadGroup(t.Context(), &redis.XReadGroupArgs{
		Group:    nc.group,
		Consumer: nc.consumer,
		Streams:  []string{streamName, ">"},
		Count:    1,
		Block:    0,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup falló: %v", err)
	}
	if len(msgs) == 0 || len(msgs[0].Messages) == 0 {
		t.Fatal("XReadGroup no retornó mensajes")
	}

	claimedMsg := msgs[0].Messages[0]

	err = nc.processMessage(t.Context(), claimedMsg)
	if err != nil {
		t.Fatalf("processMessage falló: %v", err)
	}

	if !accSender.called {
		t.Fatal("se esperaba que el sender fuera invocado")
	}
	if accSender.templateID != send_account_status_email.TemplateAccountEnabled {
		t.Errorf("TemplateID = %q, want %q", accSender.templateID, send_account_status_email.TemplateAccountEnabled)
	}

	pending, err := nc.rdb.XPending(t.Context(), streamName, nc.group).Result()
	if err != nil {
		t.Fatalf("XPending post-ACK falló: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("se esperaba PEL vacío después de ACK, count = %d", pending.Count)
	}
}

// =============================================================================
// H6.23 — TestNewNotificationConsumer_ConAccountStatus: constructor con ambos usecases
// =============================================================================

func TestNewNotificationConsumer_ConAccountStatus(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	nc := NewNotificationConsumer(client, nil, nil)
	if nc == nil {
		t.Fatal("NewNotificationConsumer retornó nil")
	}
	if nc.Name() != "notification-consumer" {
		t.Errorf("Name() = %q, want 'notification-consumer'", nc.Name())
	}
}

// =============================================================================
// H6.24 — TestOrphanRescue_ReclaimsAndProcesses: integración de rescate de huérfanos
// =============================================================================

// TestOrphanRescue_ReclaimsAndProcesses verifica que RescueOrphanedMessages
// reclama mensajes del PEL y los procesa correctamente.
func TestOrphanRescue_ReclaimsAndProcesses(t *testing.T) {
	nc, accSender, _, _ := newTestConsumerWithAccountStatus(t)

	userID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	streamName := nc.streamName

	// 1. Publicar un mensaje account_disabled al stream
	_, err := nc.rdb.XAdd(t.Context(), &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{
			"event_type":  "account_disabled",
			"user_id":     userID.String(),
			"email":       "orphan-test@example.com",
			"disabled_by": "admin-orphan",
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd falló: %v", err)
	}

	// 2. Reclamar via XREADGROUP (pone en PEL, NO hace ACK)
	msgs, err := nc.rdb.XReadGroup(t.Context(), &redis.XReadGroupArgs{
		Group:    nc.group,
		Consumer: nc.consumer,
		Streams:  []string{streamName, ">"},
		Count:    1,
		Block:    0,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup falló: %v", err)
	}
	if len(msgs) == 0 || len(msgs[0].Messages) == 0 {
		t.Fatal("XReadGroup no retornó mensajes")
	}

	// 3. Verificar que el mensaje está en PEL (no ACK'd)
	pending, err := nc.rdb.XPending(t.Context(), streamName, nc.group).Result()
	if err != nil {
		t.Fatalf("XPending falló: %v", err)
	}
	if pending.Count == 0 {
		t.Fatal("se esperaba que el mensaje estuviera en PEL después de XREADGROUP sin ACK")
	}

	// 4. Reclamar el mensaje huérfano via RescueOrphanedMessages
	//    Usar idle timeout 0 para que miniredis permita reclamar inmediatamente.
	rescued, err := eventbus.RescueOrphanedMessages(t.Context(), nc.rdb, streamName, nc.group, 0)
	if err != nil {
		t.Fatalf("RescueOrphanedMessages falló: %v", err)
	}
	if len(rescued) == 0 {
		t.Fatal("RescueOrphanedMessages debería haber reclamado al menos 1 mensaje")
	}

	// 5. Procesar el mensaje reclamado
	for _, msg := range rescued {
		err := nc.processMessage(t.Context(), msg)
		if err != nil {
			t.Fatalf("processMessage del huérfano reclamado falló: %v", err)
		}
	}

	// 6. Verificar que el sender fue invocado con el email correcto
	if !accSender.called {
		t.Fatal("se esperaba que el sender fuera invocado al procesar el huérfano")
	}
	if accSender.to != "orphan-test@example.com" {
		t.Errorf("to = %q, want %q", accSender.to, "orphan-test@example.com")
	}

	// 7. Verificar que el mensaje fue ACK'd (ya no está en PEL)
	pending, err = nc.rdb.XPending(t.Context(), streamName, nc.group).Result()
	if err != nil {
		t.Fatalf("XPending post-ACK falló: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("se esperaba PEL vacío después de procesar el huérfano, count = %d", pending.Count)
	}
}
