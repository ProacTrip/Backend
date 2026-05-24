// Tests unitarios de validación: blood_type enum.
package update_medical_profile

import (
	"testing"
)

// =============================================================================
// T-4.1: Blood type enum — table-driven tests
// =============================================================================

func TestValidBloodTypes_AllValid(t *testing.T) {
	validTypes := []string{"A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-"}
	if len(ValidBloodTypes) != 8 {
		t.Errorf("ValidBloodTypes tiene %d entradas, se esperaban 8", len(ValidBloodTypes))
	}
	for _, bt := range validTypes {
		t.Run(bt, func(t *testing.T) {
			if !ValidBloodTypes[bt] {
				t.Errorf("tipo de sangre válido %q no está en ValidBloodTypes", bt)
			}
		})
	}
}

func TestValidBloodTypes_Invalid(t *testing.T) {
	invalidTypes := []string{"XYZ", "A", "B", "AB", "O", "a+", "b-", "Apositive", ""}
	for _, bt := range invalidTypes {
		t.Run(bt, func(t *testing.T) {
			if ValidBloodTypes[bt] {
				t.Errorf("tipo de sangre inválido %q está en ValidBloodTypes", bt)
			}
		})
	}
}
