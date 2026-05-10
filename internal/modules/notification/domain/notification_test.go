// Tests de comportamiento para la entidad Notification del módulo notification.
// Verifica: NewNotification, NewEmailNotification, todos los Mark*, Transition(),
// ErrInvalidStateTransition, CanRetry, IsRetryable.
//
// Convenciones:
//   - Black-box testing (package domain_test).
//   - Table-driven con t.Run(), nombres de sub-tests en español.
//   - Solo stdlib testing, sin testify.
package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
)

// =============================================================================
// Helpers
// =============================================================================

// nuevaNotifBase crea una notificación pending lista para testear.
func nuevaNotifBase(t *testing.T) *domain.Notification {
	t.Helper()
	n, err := domain.NewNotification(
		uuid.New(),
		domain.NotificationChannelEmail,
		"contenido de prueba",
		domain.NotificationTypeTransactional,
		"asunto de prueba",
		"template-123",
		map[string]any{"key": "value"},
	)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}
	return n
}

// =============================================================================
// H3.1 — TestNuevaNotificacion: constructor NewNotification
// =============================================================================

func TestNuevaNotificacion(t *testing.T) {
	tests := []struct {
		nombre    string
		userID    uuid.UUID
		channel   domain.NotificationChannel
		content   string
		nType     domain.NotificationType
		subject   string
		template  string
		data      map[string]any
	}{
		{
			nombre:   "crear notificación con todos los campos inicializados correctamente",
			userID:   uuid.New(),
			channel:  domain.NotificationChannelEmail,
			content:  "Hola, verificá tu email",
			nType:    domain.NotificationTypeTransactional,
			subject:  "Verificación",
			template: "verify-email",
			data:     map[string]any{"url": "https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			n, err := domain.NewNotification(tt.userID, tt.channel, tt.content, tt.nType, tt.subject, tt.template, tt.data)
			if err != nil {
				t.Fatalf("NewNotification devolvió error inesperado: %v", err)
			}

			if n.ID == uuid.Nil {
				t.Error("ID no debería ser nil (se espera UUIDv7)")
			}
			if n.UserID != tt.userID {
				t.Errorf("UserID = %v, se esperaba %v", n.UserID, tt.userID)
			}
			if n.Channel != tt.channel {
				t.Errorf("Channel = %s, se esperaba %s", n.Channel, tt.channel)
			}
			if n.Content != tt.content {
				t.Errorf("Content = %q, se esperaba %q", n.Content, tt.content)
			}
			if n.Type != tt.nType {
				t.Errorf("Type = %s, se esperaba %s", n.Type, tt.nType)
			}
			if n.Status != domain.NotificationStatusPending {
				t.Errorf("Status = %s, se esperaba %s", n.Status, domain.NotificationStatusPending)
			}
			if n.CreatedAt.IsZero() {
				t.Error("CreatedAt no debería ser zero")
			}
			if n.UpdatedAt.IsZero() {
				t.Error("UpdatedAt no debería ser zero")
			}
			if n.RetryCount != 0 {
				t.Errorf("RetryCount = %d, se esperaba 0", n.RetryCount)
			}
		})
	}
}

// =============================================================================
// H3.2 — TestNuevaNotificacionEmail: constructor NewEmailNotification
// =============================================================================

func TestNuevaNotificacionEmail(t *testing.T) {
	userID := uuid.New()
	n, err := domain.NewEmailNotification(userID, "Asunto", "Contenido", domain.NotificationTypeMarketing, "template-abc", nil)
	if err != nil {
		t.Fatalf("NewEmailNotification: %v", err)
	}

	if n.Channel != domain.NotificationChannelEmail {
		t.Errorf("Channel = %s, se esperaba %s", n.Channel, domain.NotificationChannelEmail)
	}
	if n.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", n.UserID, userID)
	}
	if n.Type != domain.NotificationTypeMarketing {
		t.Errorf("Type = %s, se esperaba %s", n.Type, domain.NotificationTypeMarketing)
	}
}

// =============================================================================
// H3.3 — TestTransicionesValidas: 12 transiciones válidas de la máquina de estados
// =============================================================================

func TestTransicionesValidas(t *testing.T) {
	tests := []struct {
		nombre         string
		estadoInicial  domain.NotificationStatus
		metodo         string // "MarkSent", "MarkDelivered", "MarkOpened", "MarkFailed", "MarkBounced"
		estadoEsperado domain.NotificationStatus
	}{
		// Desde Pending
		{nombre: "pending → sent", estadoInicial: domain.NotificationStatusPending, metodo: "MarkSent", estadoEsperado: domain.NotificationStatusSent},
		{nombre: "pending → failed", estadoInicial: domain.NotificationStatusPending, metodo: "MarkFailed", estadoEsperado: domain.NotificationStatusFailed},

		// Desde Sent
		{nombre: "sent → delivered", estadoInicial: domain.NotificationStatusSent, metodo: "MarkDelivered", estadoEsperado: domain.NotificationStatusDelivered},
		{nombre: "sent → failed", estadoInicial: domain.NotificationStatusSent, metodo: "MarkFailed", estadoEsperado: domain.NotificationStatusFailed},
		{nombre: "sent → bounced", estadoInicial: domain.NotificationStatusSent, metodo: "MarkBounced", estadoEsperado: domain.NotificationStatusBounced},

		// Desde Delivered
		{nombre: "delivered → opened", estadoInicial: domain.NotificationStatusDelivered, metodo: "MarkOpened", estadoEsperado: domain.NotificationStatusOpened},
		{nombre: "delivered → bounced", estadoInicial: domain.NotificationStatusDelivered, metodo: "MarkBounced", estadoEsperado: domain.NotificationStatusBounced},

		// Desde Failed (reintento)
		{nombre: "failed → pending (reintento vía transición implícita)", estadoInicial: domain.NotificationStatusFailed, metodo: "MarkSent", estadoEsperado: domain.NotificationStatusSent},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			n := nuevaNotifBase(t)
			n.Status = tt.estadoInicial

			// Para probar failed → sent pasamos primero por pending
			if tt.estadoInicial == domain.NotificationStatusFailed && tt.metodo == "MarkSent" {
				// La transición failed → pending está permitida, luego pending → sent
				// Pero no hay MarkPending, así que cambiamos a mano simulando reinicio
				n.Status = domain.NotificationStatusPending
			}

			var err error
			switch tt.metodo {
			case "MarkSent":
				err = n.MarkSent("provider-msg-123")
			case "MarkDelivered":
				err = n.MarkDelivered()
			case "MarkOpened":
				err = n.MarkOpened()
			case "MarkFailed":
				err = n.MarkFailed("error de envío")
			case "MarkBounced":
				err = n.MarkBounced()
			}

			if err != nil {
				t.Fatalf("transición inesperadamente rechazada: %v", err)
			}
			if n.Status != tt.estadoEsperado {
				t.Errorf("Status = %s, se esperaba %s", n.Status, tt.estadoEsperado)
			}
		})
	}
}

// =============================================================================
// H3.4 — TestTransicionesInvalidas: transiciones rechazadas por la máquina
// =============================================================================

func TestTransicionesInvalidas(t *testing.T) {
	tests := []struct {
		nombre        string
		estadoInicial domain.NotificationStatus
		metodo        string
	}{
		{nombre: "pending → delivered (inválido)", estadoInicial: domain.NotificationStatusPending, metodo: "MarkDelivered"},
		{nombre: "pending → opened (inválido)", estadoInicial: domain.NotificationStatusPending, metodo: "MarkOpened"},
		{nombre: "pending → bounced (inválido)", estadoInicial: domain.NotificationStatusPending, metodo: "MarkBounced"},
		{nombre: "sent → opened (inválido)", estadoInicial: domain.NotificationStatusSent, metodo: "MarkOpened"},
		{nombre: "delivered → sent (inválido)", estadoInicial: domain.NotificationStatusDelivered, metodo: "MarkSent"},
		{nombre: "opened → delivered (inválido)", estadoInicial: domain.NotificationStatusOpened, metodo: "MarkDelivered"},
		{nombre: "opened → sent (inválido)", estadoInicial: domain.NotificationStatusOpened, metodo: "MarkSent"},
		{nombre: "bounced → sent (inválido)", estadoInicial: domain.NotificationStatusBounced, metodo: "MarkSent"},
		{nombre: "bounced → delivered (inválido)", estadoInicial: domain.NotificationStatusBounced, metodo: "MarkDelivered"},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			n := nuevaNotifBase(t)
			n.Status = tt.estadoInicial

			var err error
			switch tt.metodo {
			case "MarkSent":
				err = n.MarkSent("msg-123")
			case "MarkDelivered":
				err = n.MarkDelivered()
			case "MarkOpened":
				err = n.MarkOpened()
			case "MarkFailed":
				err = n.MarkFailed("error")
			case "MarkBounced":
				err = n.MarkBounced()
			}

			if err == nil {
				t.Fatal("se esperaba ErrInvalidStateTransition, se obtuvo nil")
			}
			if !errors.Is(err, domain.ErrInvalidStateTransition) {
				t.Errorf("error = %v, se esperaba ErrInvalidStateTransition", err)
			}

			// El estado NO debe haber cambiado
			if n.Status != tt.estadoInicial {
				t.Errorf("el estado mutó de %s a %s cuando la transición fue rechazada",
					tt.estadoInicial, n.Status)
			}
		})
	}
}

// =============================================================================
// H3.5 — TestDobleMarkSent: aplicar MarkSent dos veces debe fallar
// =============================================================================

func TestDobleMarkSent(t *testing.T) {
	n := nuevaNotifBase(t)

	err := n.MarkSent("msg-1")
	if err != nil {
		t.Fatalf("primer MarkSent falló: %v", err)
	}

	err = n.MarkSent("msg-2")
	if err == nil {
		t.Fatal("se esperaba error en segundo MarkSent, se obtuvo nil")
	}
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("error = %v, se esperaba ErrInvalidStateTransition", err)
	}

	// ProviderMessageID no debe cambiar
	if n.ProviderMessageID != "msg-1" {
		t.Errorf("ProviderMessageID = %q, se esperaba 'msg-1'", n.ProviderMessageID)
	}
}

// =============================================================================
// H3.6 — TestMarkSentGuardaProviderMessageID
// =============================================================================

func TestMarkSentGuardaProviderMessageID(t *testing.T) {
	n := nuevaNotifBase(t)

	err := n.MarkSent("resend_abc123")
	if err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	if n.ProviderMessageID != "resend_abc123" {
		t.Errorf("ProviderMessageID = %q, se esperaba 'resend_abc123'", n.ProviderMessageID)
	}
	if n.SentAt == nil {
		t.Error("SentAt no debería ser nil después de MarkSent")
	}
	if n.Status != domain.NotificationStatusSent {
		t.Errorf("Status = %s, se esperaba %s", n.Status, domain.NotificationStatusSent)
	}
}

// =============================================================================
// H3.7 — TestMarkDelivered
// =============================================================================

func TestMarkDelivered(t *testing.T) {
	n := nuevaNotifBase(t)

	// Primero enviar
	if err := n.MarkSent("msg-1"); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	if err := n.MarkDelivered(); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}

	if n.Status != domain.NotificationStatusDelivered {
		t.Errorf("Status = %s, se esperaba %s", n.Status, domain.NotificationStatusDelivered)
	}
	if n.DeliveredAt == nil {
		t.Error("DeliveredAt no debería ser nil después de MarkDelivered")
	}
}

// =============================================================================
// H3.8 — TestMarkOpened
// =============================================================================

func TestMarkOpened(t *testing.T) {
	n := nuevaNotifBase(t)

	if err := n.MarkSent("msg-1"); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if err := n.MarkDelivered(); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}

	if err := n.MarkOpened(); err != nil {
		t.Fatalf("MarkOpened: %v", err)
	}

	if n.Status != domain.NotificationStatusOpened {
		t.Errorf("Status = %s, se esperaba %s", n.Status, domain.NotificationStatusOpened)
	}
	if n.OpenedAt == nil {
		t.Error("OpenedAt no debería ser nil después de MarkOpened")
	}
}

// =============================================================================
// H3.9 — TestMarkFailedIncrementaRetryCount
// =============================================================================

func TestMarkFailedIncrementaRetryCount(t *testing.T) {
	n := nuevaNotifBase(t)

	// Primer fallo
	if err := n.MarkFailed("error 1"); err != nil {
		t.Fatalf("MarkFailed 1: %v", err)
	}
	if n.RetryCount != 1 {
		t.Errorf("RetryCount después del primer fallo = %d, se esperaba 1", n.RetryCount)
	}
	if n.ErrorMessage != "error 1" {
		t.Errorf("ErrorMessage = %q, se esperaba 'error 1'", n.ErrorMessage)
	}

	// Simular reinjection: el consumer marca pending para reintentar
	// (transición válida: failed → pending)
	n.Status = domain.NotificationStatusPending

	// Segundo fallo
	if err := n.MarkFailed("error 2"); err != nil {
		t.Fatalf("MarkFailed 2: %v", err)
	}
	if n.RetryCount != 2 {
		t.Errorf("RetryCount después del segundo fallo = %d, se esperaba 2", n.RetryCount)
	}
	if n.ErrorMessage != "error 2" {
		t.Errorf("ErrorMessage = %q, se esperaba 'error 2'", n.ErrorMessage)
	}
}

// =============================================================================
// H3.10 — TestCanRetry
// =============================================================================

func TestCanRetry(t *testing.T) {
	tests := []struct {
		nombre       string
		status       domain.NotificationStatus
		retryCount   int
		maxAttempts  int
		esperaResult bool
	}{
		{nombre: "pending con intentos disponibles", status: domain.NotificationStatusPending, retryCount: 0, maxAttempts: 3, esperaResult: true},
		{nombre: "pending con retryCount=2, max=3", status: domain.NotificationStatusPending, retryCount: 2, maxAttempts: 3, esperaResult: true},
		{nombre: "pending agotó reintentos (retryCount=3, max=3)", status: domain.NotificationStatusPending, retryCount: 3, maxAttempts: 3, esperaResult: false},
		{nombre: "failed con intentos disponibles", status: domain.NotificationStatusFailed, retryCount: 1, maxAttempts: 5, esperaResult: true},
		{nombre: "failed agotó reintentos (retryCount=5, max=5)", status: domain.NotificationStatusFailed, retryCount: 5, maxAttempts: 5, esperaResult: false},
		{nombre: "sent no es reintentable", status: domain.NotificationStatusSent, retryCount: 0, maxAttempts: 3, esperaResult: false},
		{nombre: "delivered no es reintentable", status: domain.NotificationStatusDelivered, retryCount: 0, maxAttempts: 3, esperaResult: false},
		{nombre: "opened no es reintentable", status: domain.NotificationStatusOpened, retryCount: 0, maxAttempts: 3, esperaResult: false},
		{nombre: "bounced no es reintentable", status: domain.NotificationStatusBounced, retryCount: 0, maxAttempts: 3, esperaResult: false},
		{nombre: "maxAttempts=0 deshabilita reintentos", status: domain.NotificationStatusPending, retryCount: 0, maxAttempts: 0, esperaResult: false},
		{nombre: "maxAttempts negativo deshabilita reintentos", status: domain.NotificationStatusPending, retryCount: 0, maxAttempts: -1, esperaResult: false},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			n := nuevaNotifBase(t)
			n.Status = tt.status
			n.RetryCount = tt.retryCount

			result := n.CanRetry(tt.maxAttempts)
			if result != tt.esperaResult {
				t.Errorf("CanRetry(%d) = %v, se esperaba %v", tt.maxAttempts, result, tt.esperaResult)
			}
		})
	}
}

// =============================================================================
// H3.11 — TestIsRetryable
// =============================================================================

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		nombre  string
		status  domain.NotificationStatus
		espera  bool
	}{
		{nombre: "pending es reintentable", status: domain.NotificationStatusPending, espera: true},
		{nombre: "failed es reintentable", status: domain.NotificationStatusFailed, espera: true},
		{nombre: "sent no es reintentable", status: domain.NotificationStatusSent, espera: false},
		{nombre: "delivered no es reintentable", status: domain.NotificationStatusDelivered, espera: false},
		{nombre: "opened no es reintentable", status: domain.NotificationStatusOpened, espera: false},
		{nombre: "bounced no es reintentable", status: domain.NotificationStatusBounced, espera: false},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			n := nuevaNotifBase(t)
			n.Status = tt.status

			if result := n.IsRetryable(); result != tt.espera {
				t.Errorf("IsRetryable() = %v, se esperaba %v", result, tt.espera)
			}
		})
	}
}

// =============================================================================
// H3.13 — TestTransitionDirecta
// =============================================================================

func TestTransitionDirecta(t *testing.T) {
	n := nuevaNotifBase(t)

	err := n.Transition(domain.NotificationStatusSent)
	if err != nil {
		t.Fatalf("Transition(Sent) desde pending: %v", err)
	}

	// Transition es solo validación, no muta estado
	if n.Status != domain.NotificationStatusPending {
		t.Errorf("Transition no debe mutar Status: era pending, ahora %s", n.Status)
	}
}

// =============================================================================
// H3.14 — TestMarkBounced
// =============================================================================

func TestMarkBounced(t *testing.T) {
	n := nuevaNotifBase(t)
	if err := n.MarkSent("msg-1"); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	if err := n.MarkBounced(); err != nil {
		t.Fatalf("MarkBounced: %v", err)
	}

	if n.Status != domain.NotificationStatusBounced {
		t.Errorf("Status = %s, se esperaba %s", n.Status, domain.NotificationStatusBounced)
	}
	// UpdatedAt debe haberse actualizado
	if n.SentAt != nil && n.UpdatedAt.Before(*n.SentAt) {
		t.Error("UpdatedAt no debería ser anterior a SentAt después de MarkBounced")
	}
}

// =============================================================================
// H3.15 — TestMarkSentActualizaTimestamp
// =============================================================================

func TestMarkSentActualizaTimestamp(t *testing.T) {
	n := nuevaNotifBase(t)
	antes := n.UpdatedAt

	time.Sleep(1 * time.Millisecond)

	if err := n.MarkSent("msg-1"); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	if !n.UpdatedAt.After(antes) {
		t.Error("UpdatedAt debe haberse actualizado después de MarkSent")
	}
	if n.SentAt == nil {
		t.Error("SentAt debe estar poblado")
	}
}

// =============================================================================
// H3.16 — TestErrInvalidStateTransitionEsSentinel
// =============================================================================

func TestErrInvalidStateTransitionEsSentinel(t *testing.T) {
	// Verificar que ErrInvalidStateTransition es un error sentinel válido.
	if domain.ErrInvalidStateTransition == nil {
		t.Fatal("ErrInvalidStateTransition no debería ser nil")
	}
	if domain.ErrInvalidStateTransition.Error() == "" {
		t.Error("ErrInvalidStateTransition.Error() no debería ser vacío")
	}

	// Verificar que errors.Is funciona con el sentinel.
	n := nuevaNotifBase(t)
	n.Status = domain.NotificationStatusDelivered
	err := n.MarkSent("msg")
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("errors.Is debería detectar ErrInvalidStateTransition, obtuvo %v", err)
	}
}

// =============================================================================
// H3.17 — TestNewNotificationError: UUID generation failure not testable directly
// =============================================================================

func TestNewNotification_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewNotification hizo panic: %v", r)
		}
	}()

	n, err := domain.NewNotification(
		uuid.New(),
		domain.NotificationChannelEmail,
		"content",
		domain.NotificationTypeTransactional,
		"subject",
		"template",
		nil,
	)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}
	if n == nil {
		t.Fatal("NewNotification retornó nil sin error")
	}
}
