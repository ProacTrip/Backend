// DTO de respuesta para DELETE /v1/user/favorites/:favorite_id.
package delete_favorite

// DeleteFavoriteResponse es la respuesta del endpoint de eliminación de favorito.
type DeleteFavoriteResponse struct {
	Message string `json:"message"`
}
