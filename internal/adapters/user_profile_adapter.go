// UserProfileAdapter adapta shared/user.GetProfilePrefs (Dragonfly hash)
// al puerto search/domain.UserProfilePort para inyección en handlers/usecases.
package adapters

import (
	"context"

	"github.com/redis/go-redis/v9"

	searchdomain "github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/shared/user"
)

// UserProfileAdapter implements searchdomain.UserProfilePort by delegating to
// shared/user.GetProfilePrefs which reads the Dragonfly hash user:prefs:{userID}.
type UserProfileAdapter struct {
	rdb *redis.Client
}

// Compile-time guard: ensure UserProfileAdapter satisfies searchdomain.UserProfilePort.
var _ searchdomain.UserProfilePort = (*UserProfileAdapter)(nil)

// NewUserProfileAdapter creates a new adapter backed by the given Dragonfly client.
func NewUserProfileAdapter(rdb *redis.Client) *UserProfileAdapter {
	return &UserProfileAdapter{rdb: rdb}
}

// GetPreferences resolves currency and language from the user's cached profile prefs.
// Returns empty strings for anonymous users (userID="") or cache miss — the caller
// is responsible for falling back to config defaults.
func (a *UserProfileAdapter) GetPreferences(ctx context.Context, userID string) (string, string, error) {
	if userID == "" || a.rdb == nil {
		return "", "", nil
	}
	prefs, err := user.GetProfilePrefs(ctx, a.rdb, userID)
	if err != nil {
		return "", "", err
	}
	if prefs == nil {
		return "", "", nil
	}
	return prefs.Currency, prefs.Language, nil
}
