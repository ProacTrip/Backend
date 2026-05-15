// DTO de respuesta para DELETE /v1/user/saved-searches/:search_id.
package delete_saved_search

// DeleteSavedSearchResponse es la respuesta del endpoint de eliminación de búsqueda guardada.
type DeleteSavedSearchResponse struct {
	Message string `json:"message"`
}
