// DTO de respuesta para POST /v1/user/favorites.
package add_favorite

// Response es la respuesta del endpoint de creación de favorito.
type Response struct {
	FavoriteID string `json:"favorite_id"`
	Message    string `json:"message"`
}
