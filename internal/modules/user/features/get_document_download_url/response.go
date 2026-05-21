// DTO de respuesta para GET /v1/user/profile/documents/:document_id/download-url.
// Alineado con docs/USER_API.md § Get Document Download URL.
package get_document_download_url

// DownloadURLResponse contiene la URL prefirmada y metadata.
type DownloadURLResponse struct {
	DownloadURL string `json:"download_url"`
	ExpiresAt   string `json:"expires_at"`
	FileName    string `json:"file_name"`
}
