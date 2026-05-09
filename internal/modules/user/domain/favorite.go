// Domain: Favoritos del usuario.
// Reemplaza wishlists/wishlist_items con un modelo simplificado.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Enums
// =============================================================================

// FavoriteEntityType representa el tipo de entidad marcada como favorita.
type FavoriteEntityType string

const (
	FavoriteEntityHotel       FavoriteEntityType = "hotel"
	FavoriteEntityFlight      FavoriteEntityType = "flight"
	FavoriteEntityAirport     FavoriteEntityType = "airport"
	FavoriteEntityAirline     FavoriteEntityType = "airline"
	FavoriteEntityHotelChain  FavoriteEntityType = "hotel_chain"
	FavoriteEntityCountry     FavoriteEntityType = "country"
	FavoriteEntityDestination FavoriteEntityType = "destination"
	FavoriteEntityActivity    FavoriteEntityType = "activity"
)

var validFavoriteEntityTypes = map[FavoriteEntityType]bool{
	FavoriteEntityHotel:      true,
	FavoriteEntityFlight:     true,
	FavoriteEntityAirport:    true,
	FavoriteEntityAirline:    true,
	FavoriteEntityHotelChain: true,
	FavoriteEntityCountry:    true,
	FavoriteEntityDestination: true,
	FavoriteEntityActivity:   true,
}

// IsValidFavoriteEntityType verifica si el string es un tipo de entidad favorita válido.
func IsValidFavoriteEntityType(t string) bool {
	return validFavoriteEntityTypes[FavoriteEntityType(t)]
}

// =============================================================================
// Favorite — Entidad favorita del usuario
// =============================================================================

// Favorite representa un elemento marcado como favorito por el usuario.
// Reemplaza la tabla wishlists y wishlist_items del modelo anterior.
// Alineado con la migración 002 user_favorites.
type Favorite struct {
	ID         uuid.UUID         `json:"id"`
	UserID     uuid.UUID         `json:"user_id"`
	EntityID   uuid.UUID         `json:"entity_id"`
	EntityType FavoriteEntityType `json:"entity_type"`
	Title      string            `json:"title"`
	Notes      *string           `json:"notes,omitzero"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// NewFavorite crea un nuevo favorito.
func NewFavorite(userID, entityID uuid.UUID, entityType FavoriteEntityType, title string) *Favorite {
	now := time.Now()
	return &Favorite{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     userID,
		EntityID:   entityID,
		EntityType: entityType,
		Title:      title,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// SetNotes establece notas opcionales en el favorito.
func (f *Favorite) SetNotes(notes *string) {
	f.Notes = notes
	f.UpdatedAt = time.Now()
}
