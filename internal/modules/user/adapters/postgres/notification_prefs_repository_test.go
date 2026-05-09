// Test: NotificationPrefsRepository interface compliance (compile-time).
package postgres

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

func TestNotificationPrefsRepositoryImplementsInterface(t *testing.T) {
	var _ domain.NotificationPrefsRepository = (*NotificationPrefsRepository)(nil) //nolint:unused
}

func TestNotificationPrefsRepoMethodsExist(t *testing.T) {
	r := &NotificationPrefsRepository{db: nil}
	_ = r
	t.Log("NotificationPrefsRepository has all required methods")
}
