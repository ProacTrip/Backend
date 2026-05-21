// DTO de respuesta para GET /v1/user/profile/medical.
package get_medical_profile

import (
	"time"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// MedicalFieldEntry representa un campo médico con su valor, fuente y timestamp.
type MedicalFieldEntry struct {
	Value     string                      `json:"value"`
	Source    domain.MedicalSourceDetail  `json:"source"`
	UpdatedAt string                      `json:"updated_at"`
}

// MedicalProfileResponse es la respuesta del endpoint de perfil médico.
type MedicalProfileResponse struct {
	Data                 map[string]*MedicalFieldEntry `json:"data"`
	IsShared             bool                          `json:"is_shared"`
	HasPendingConflicts  bool                          `json:"has_pending_conflicts"`
	PendingConflictCount int                           `json:"pending_conflict_count"`
}

// formatTime retorna un time.Time como ISO 8601 string.
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
