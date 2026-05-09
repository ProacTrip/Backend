// DTO de respuesta para confirm_avatar_upload.
package confirm_avatar_upload

// Response contiene la respuesta de POST /v1/user/profile/avatar/confirm.
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
