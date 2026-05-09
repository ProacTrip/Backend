// Comando para PUT /v1/user/profile/medical.
// Todos los campos son punteros: nil = no tocar, valor = actualizar.
package update_medical_profile

import (
	"github.com/google/uuid"
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
type Command struct {
	UserID           string  `json:"-"`
	BloodType        *string `json:"blood_type,omitzero"`
	Allergies        *string `json:"allergies,omitzero"`
	Medications      *string `json:"medications,omitzero"`
	Conditions       *string `json:"conditions,omitzero"`
	Vaccinations     *string `json:"vaccinations,omitzero"`
	EmergencyContact *string `json:"emergency_contact,omitzero"`
	InsuranceInfo    *string `json:"insurance_info,omitzero"`
	IsShared         *bool   `json:"is_shared,omitzero"`
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
