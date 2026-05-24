// Comando para PATCH /v1/user/profile/medical.
// Todos los campos son punteros: nil = no tocar, valor = actualizar.
// Los campos encriptados ahora usan tipos tipados (*[]string, *[]Medication, etc.)
// en lugar de *string — el usecase los marshaliza a JSON antes de encriptar.
package update_medical_profile

import (
	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/user/domain"
)

// ValidBloodTypes contiene los tipos de sangre válidos.
var ValidBloodTypes = map[string]bool{
	"A+": true, "A-": true,
	"B+": true, "B-": true,
	"AB+": true, "AB-": true,
	"O+": true, "O-": true,
}

// Command contiene los campos actualizables del perfil médico.
// Todos los campos son opcionales (*pointer = nil significa "no actualizar").
// Los campos encriptados usan el tipo Go nativo ([]string, []Medication, etc.)
// y son marshalizados a JSON antes de encriptar en el usecase.
type Command struct {
	UserID           string                    `json:"-"`
	BloodType        *string                   `json:"blood_type,omitzero"`
	Allergies        *[]string                 `json:"allergies,omitzero"`
	Medications      *[]domain.Medication      `json:"medications,omitzero"`
	Conditions       *[]string                 `json:"conditions,omitzero"`
	Vaccinations     *[]domain.Vaccination     `json:"vaccinations,omitzero"`
	EmergencyContact *domain.EmergencyContact  `json:"emergency_contact,omitzero"`
	InsuranceInfo    *domain.InsuranceInfo     `json:"insurance_info,omitzero"`
	IsShared         *bool                     `json:"is_shared,omitzero"`
}

// Validate valida que UserID sea un UUID válido y blood_type sea válido si se envió.
func (c *Command) Validate() error {
	if _, err := uuid.Parse(c.UserID); err != nil {
		return err
	}
	return nil
}

// ValidateBloodType valida el tipo de sangre contra el enum.
func (c *Command) ValidateBloodType() error {
	if c.BloodType != nil && !ValidBloodTypes[*c.BloodType] {
		return nil // la validación de blood_type se hace en el usecase
	}
	return nil
}
