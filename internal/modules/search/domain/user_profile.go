// UserProfilePort — puerto de acceso a preferencias de usuario para search.
// Parte de la arquitectura hexagonal: el handler/usecase depende de la abstracción,
// el adapter (wiring layer) implementa la conexión concreta a Dragonfly.
package domain

import "context"

// UserProfilePort provides access to user preferences for search personalization.
// Implemented by an adapter in the wiring layer to avoid direct module imports.
type UserProfilePort interface {
	GetPreferences(ctx context.Context, userID string) (currency string, language string, err error)
}
