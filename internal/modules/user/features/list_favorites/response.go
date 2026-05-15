// DTO de respuesta para GET /v1/user/favorites.
package list_favorites

// FavoriteItem representa un favorito en la respuesta de listado.
type FavoriteItem struct {
	ID         string  `json:"id"`
	EntityID   string  `json:"entity_id"`
	EntityType string  `json:"entity_type"`
	Title      string  `json:"title"`
	Notes      *string `json:"notes,omitzero"`
	CreatedAt  string  `json:"created_at"`
}

// Response agrupa la lista de favoritos del usuario.
type Response struct {
	Favorites []FavoriteItem `json:"favorites"`
}
