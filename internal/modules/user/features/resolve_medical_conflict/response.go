// DTO de respuesta para POST /v1/user/profile/medical-conflicts/:conflict_id/resolve.
// Alineado con docs/USER_API.md § Resolve Medical Conflict.
package resolve_medical_conflict

// ResolveResponse es la respuesta del endpoint.
type ResolveResponse struct {
	Message string `json:"message"`
}
