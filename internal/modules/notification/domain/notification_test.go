// Tests de comportamiento para la entidad Notification del módulo notification.
// Verifica: NewEmailNotification.
//
// Convenciones:
//   - Black-box testing (package domain_test).
//   - Table-driven con t.Run(), nombres de sub-tests en español.
//   - Solo stdlib testing, sin testify.
package domain_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/notification/domain"
)

// =============================================================================
// TestNuevaNotificacionEmail: constructor NewEmailNotification
// =============================================================================

func TestNuevaNotificacionEmail(t *testing.T) {
	userID := uuid.New()
	n, err := domain.NewEmailNotification(userID, "template-abc")
	if err != nil {
		t.Fatalf("NewEmailNotification: %v", err)
	}

	if n.ID == uuid.Nil {
		t.Error("ID no debería ser nil (se espera UUIDv7)")
	}
	if n.UserID != userID {
		t.Errorf("UserID = %v, se esperaba %v", n.UserID, userID)
	}
	if n.TemplateCode != "template-abc" {
		t.Errorf("TemplateCode = %q, se esperaba %q", n.TemplateCode, "template-abc")
	}
	if n.SentAt != nil {
		t.Error("SentAt debería ser nil en una notificación nueva")
	}
	if n.CreatedAt.IsZero() {
		t.Error("CreatedAt no debería ser zero")
	}
	if n.UpdatedAt.IsZero() {
		t.Error("UpdatedAt no debería ser zero")
	}
}

// =============================================================================
// TestNotificationTypeTransactional: verificar que la constante existe
// =============================================================================

func TestNotificationTypeTransactional(t *testing.T) {
	if domain.NotificationTypeTransactional != "transactional" {
		t.Errorf("NotificationTypeTransactional = %q, se esperaba 'transactional'",
			domain.NotificationTypeTransactional)
	}
}
