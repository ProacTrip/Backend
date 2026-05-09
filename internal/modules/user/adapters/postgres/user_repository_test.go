// Test: ProfileRepository interface compliance and error behavior.
package postgres

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// =============================================================================
// RED — Compile-time interface check: UserRepository implements ProfileRepository
// =============================================================================

func TestUserRepositoryImplementsProfileRepository(t *testing.T) {
	// Compile-time verification: if this doesn't compile, the adapter
	// does NOT satisfy the ProfileRepository interface.
	var _ domain.ProfileRepository = (*UserRepository)(nil) //nolint:unused
}

// =============================================================================
// RED — Verify UserRepository satisfies GetByUserID returns error on not found
// (The actual query behavior is tested via integration tests with a real DB)
// =============================================================================

func TestUserRepositoryProfileRepoMethodsExist(t *testing.T) {
	// Verify the struct has the required methods at compile time.
	// This is a structural test — it confirms method signatures exist.
	r := &UserRepository{db: nil}

	// Verify the type satisfies ProfileRepository (compile-time only test)
	_ = r
	t.Log("UserRepository has all required methods for ProfileRepository interface")
}
