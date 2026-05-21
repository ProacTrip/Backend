// DTO de respuesta para GET /v1/user/profile/medical-conflicts/:conflict_id.
// Alineado con docs/USER_API.md § Get Medical Conflict.
package get_medical_conflict

// ConflictSource representa el origen de un conflicto médico.
type ConflictSource struct {
	Type       string  `json:"type"`
	DocumentID *string `json:"document_id,omitzero"`
	FileName   *string `json:"file_name,omitzero"`
}

// MedicalConflictResponse es la respuesta del endpoint de detalle de conflicto.
type MedicalConflictResponse struct {
	ID           string         `json:"id"`
	Field        string         `json:"field"`
	CurrentValue *string        `json:"current_value,omitzero"`
	ProposedValue string        `json:"proposed_value"`
	Source       ConflictSource `json:"source"`
	Status       string         `json:"status"`
	SuggestedAt  string         `json:"suggested_at"`
	ExpiresAt    string         `json:"expires_at"`
	ResolvedAt   *string        `json:"resolved_at,omitzero"`
	Resolution   *string        `json:"resolution,omitzero"`
}
