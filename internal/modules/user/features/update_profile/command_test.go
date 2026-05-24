// Tests unitarios de validación: E.164 phone regex, IsValidPhone.
package update_profile

import (
	"testing"
)

// =============================================================================
// T-4.1: E.164 phone regex — table-driven tests
// =============================================================================

func TestIsValidPhone_ValidE164(t *testing.T) {
	validPhones := []string{
		"+5491123456789", // Argentina
		"+12025550123",   // USA
		"+8613800138000", // China
		"+34123456789",   // España
		"+447911123456",  // UK
		"+541112345678",  // Buenos Aires fijo
		"+11",            // mínimo E.164 válido: +, código país [1-9], al menos 1 dígito más
		"+123456789012345", // máximo 15 dígitos total (+ más 14 dígitos)
	}
	for _, phone := range validPhones {
		t.Run(phone, func(t *testing.T) {
			if !IsValidPhone(&phone) {
				t.Errorf("teléfono válido %q fue rechazado", phone)
			}
		})
	}
}

func TestIsValidPhone_InvalidE164(t *testing.T) {
	invalidPhones := []struct {
		phone string
		why   string
	}{
		{"1123456789", "sin +"},
		{"++5491123456789", "doble +"},
		{"+0", "empieza con 0 después del +"},
		{"+", "solo +"},
		{"+54 911 23456789", "espacios"},
		{"+54-911-23456789", "guiones"},
		{"12345", "sin + ni código de país"},
		{"+5491123456789a", "letras"},
	}
	for _, tc := range invalidPhones {
		t.Run(tc.why, func(t *testing.T) {
			if IsValidPhone(&tc.phone) {
				t.Errorf("teléfono inválido %q debería haber sido rechazado (%s)", tc.phone, tc.why)
			}
		})
	}
}

func TestIsValidPhone_NilAndEmpty(t *testing.T) {
	// nil = no tocar → válido (skip validation)
	if !IsValidPhone(nil) {
		t.Error("phone nil debería ser válido (skip validation)")
	}
	// "" = limpiar → válido (clear field, no validation needed)
	empty := ""
	if !IsValidPhone(&empty) {
		t.Error("phone vacío debería ser válido (clear field)")
	}
}
