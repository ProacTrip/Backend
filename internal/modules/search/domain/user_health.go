// UserHealthPort — puerto de acceso al perfil médico, preferencias de viaje,
// y documentos del usuario para inyección en el contexto del sistema de IA.
// Parte de la arquitectura hexagonal: el handler/usecase depende de la abstracción,
// el adapter (wiring layer) implementa la conexión concreta al módulo user.
package domain

import "context"

// =============================================================================
// DTOs — transportan datos médicos y de viaje al contexto del sistema de IA
// =============================================================================

// MedicalAIContext agrupa los datos del perfil médico relevantes para la IA.
// Los campos nombrados permiten al modelo razonar sobre categorías conocidas.
// Extra preserva campos no mapeados para flexibilidad futura.
type MedicalAIContext struct {
	Allergies    []string              `json:"allergies,omitzero"`
	Conditions   []string              `json:"conditions,omitzero"`
	Medications  []string              `json:"medications,omitzero"`
	Vaccinations []string              `json:"vaccinations,omitzero"`
	BloodType    string                `json:"blood_type,omitzero"`
	Extra        map[string]interface{} `json:"extra,omitzero"`
}

// TravelAIContext agrupa las preferencias de viaje relevantes para la IA.
type TravelAIContext struct {
	PreferredClass     string   `json:"preferred_class,omitzero"`
	SeatPreference     string   `json:"seat_preference,omitzero"`
	MealPreference     string   `json:"meal_preference,omitzero"`
	SpecialAssistance  []string `json:"special_assistance,omitzero"`
	AvoidLayovers      bool     `json:"avoid_layovers,omitzero"`
	MaxLayoverDuration int      `json:"max_layover_duration,omitzero"`
}

// DocumentContext agrupa los datos extraídos de un documento de viaje (pasaporte/visa).
type DocumentContext struct {
	Type           string                 `json:"type"`
	Number         string                 `json:"number,omitzero"`
	IssuingCountry string                 `json:"issuing_country,omitzero"`
	Nationality    string                 `json:"nationality,omitzero"`
	ValidUntil     string                 `json:"valid_until,omitzero"`
	Extra          map[string]interface{} `json:"extra,omitzero"`
}

// MedicalAlert representa una alerta de salud o viaje generada por la IA.
type MedicalAlert struct {
	Level   string `json:"level"`   // "info", "warning", "danger"
	Type    string `json:"type"`    // "allergy", "medication", "vaccination", "condition", "travel", "document"
	Message string `json:"message"` // texto descriptivo de la alerta
}

// =============================================================================
// UserHealthPort — puerto para datos de salud y viaje del usuario
// =============================================================================

// UserHealthPort provides access to user health, travel preferences, and
// document data for injection into the AI system prompt.
// Implemented by an adapter in the wiring layer to avoid direct module imports.
type UserHealthPort interface {
	GetMedicalContext(ctx context.Context, userID string) (*MedicalAIContext, error)
	GetTravelPreferences(ctx context.Context, userID string) (*TravelAIContext, error)
	GetDocumentContext(ctx context.Context, userID string) ([]DocumentContext, error)
	GetNationality(ctx context.Context, userID string) string
}
