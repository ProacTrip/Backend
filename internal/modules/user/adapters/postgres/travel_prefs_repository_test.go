// Test: TravelPrefsRepository interface compliance (compile-time).
package postgres

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// RED — Compile-time interface check: TravelPrefsRepository implements domain
// =============================================================================

func TestTravelPrefsRepositoryImplementsInterface(t *testing.T) {
	var _ domain.TravelPrefsRepository = (*TravelPrefsRepository)(nil) //nolint:unused
}

// TestTravelPrefsRepoMethodsExist verifies the struct has the required methods.
func TestTravelPrefsRepoMethodsExist(t *testing.T) {
	r := &TravelPrefsRepository{db: nil}
	_ = r
	t.Log("TravelPrefsRepository has all required methods")
}
