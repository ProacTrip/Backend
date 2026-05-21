// DTO de respuesta para upload_avatar.
package upload_avatar

import "time"

// Response contiene la respuesta de POST /v1/user/profile/avatar.
type Response struct {
	UploadURL  string    `json:"upload_url"`
	StorageKey string    `json:"storage_key"`
	ExpiresAt  time.Time `json:"expires_at"`
	EventsURL  string    `json:"events_url"`
	Message    string    `json:"message"`
}
