// DTO de respuesta para GET /v1/user/profile/medical-conflicts.
// Alineado con docs/USER_API.md § List Medical Conflicts.
package list_medical_conflicts

// ConflictSource representa el origen de un conflicto.
type ConflictSource struct {
	Type       string  `json:"type"`
	DocumentID *string `json:"document_id,omitzero"`
	FileName   *string `json:"file_name,omitzero"`
}

// ConflictEntry representa un conflicto médico individual.
type ConflictEntry struct {
	ID            string         `json:"id"`
	Field         string         `json:"field"`
	CurrentValue  *string        `json:"current_value,omitzero"`
	ProposedValue string         `json:"proposed_value"`
	Source        ConflictSource `json:"source"`
	Status        string         `json:"status"`
	SuggestedAt   string         `json:"suggested_at"`
	ExpiresAt     string         `json:"expires_at"`
}

// ListMedicalConflictsResponse es la respuesta del endpoint de conflictos médicos.
type ListMedicalConflictsResponse struct {
	Conflicts []ConflictEntry `json:"conflicts"`
}
