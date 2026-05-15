// Tests para errores de dominio del pipeline de discovery.
// Verifica que los sentinels estén definidos, tengan código y mensaje,
// y que sean distintos entre sí.
package domain

import (
	"errors"
	"testing"
)

func TestDiscoveryErrors_Exist(t *testing.T) {
	tests := []struct {
		name    string
		sentinel error
		code    string
		desc    string
	}{
		{
			name:    "ErrDiscoveryDisabled",
			sentinel: ErrDiscoveryDisabled,
			code:    "DISCOVERY_DISABLED",
			desc:    "debe contener 'discovery'",
		},
		{
			name:    "ErrNoCandidatesFound",
			sentinel: ErrNoCandidatesFound,
			code:    "NO_CANDIDATES",
			desc:    "debe contener 'candidatos' o 'destinos'",
		},
		{
			name:    "ErrClarifyMaxRounds",
			sentinel: ErrClarifyMaxRounds,
			code:    "CLARIFY_MAX_ROUNDS",
			desc:    "debe contener 'aclaración' o 'preguntas'",
		},
	}

	allErrors := []error{
		ErrInvalidTripType, ErrMissingRequiredField, ErrInvalidParameterRange,
		ErrProviderUnavailable, ErrProviderBadRequest, ErrProviderError,
		ErrNoResults, ErrTokenInvalid, ErrTokenRequired, ErrCacheFailed,
		ErrRateLimitExceeded,
		ErrAIUnavailable, ErrAIParseFailure, ErrConversationNotFound,
		ErrTurnLimitExceeded,
		ErrBookingTokenExpired, ErrPropertyNotFound,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// El sentinel debe estar definido (no nil)
			if tc.sentinel == nil {
				t.Fatalf("%s es nil — no fue definido", tc.name)
			}

			msg := tc.sentinel.Error()

			// Debe tener formato "CODE: descripción"
			if len(msg) == 0 {
				t.Fatal("el mensaje de error no puede estar vacío")
			}
			if msg[:len(tc.code)] != tc.code {
				t.Errorf("el código de error no coincide: se esperaba %q, se obtuvo %q",
					tc.code, msg[:len(tc.code)])
			}

			// Las colecciones de errores deben poder referenciarse con errors.Is
			if !errors.Is(tc.sentinel, tc.sentinel) {
				t.Errorf("errors.Is(%s, %s) debería ser true — el sentinel no es auto-referenciable",
					tc.name, tc.name)
			}

			// No debe coincidir con ningún error existente
			for _, other := range allErrors {
				if other != nil && errors.Is(other, tc.sentinel) {
					t.Errorf("errors.Is(existingErr, %s) es true — el nuevo sentinel colisiona con uno existente", tc.name)
				}
				if errors.Is(tc.sentinel, other) {
					t.Errorf("errors.Is(%s, existingErr) es true — el nuevo sentinel colisiona con uno existente", tc.name)
				}
			}
		})
	}
}

func TestDiscoveryErrors_Distinct(t *testing.T) {
	// Cada par de errores nuevos debe ser distinto
	errs := []error{
		ErrDiscoveryDisabled,
		ErrNoCandidatesFound,
		ErrClarifyMaxRounds,
	}

	for i := range errs {
		for j := range errs {
			if i != j && errors.Is(errs[i], errs[j]) {
				t.Errorf("errors.Is(err[%d], err[%d]) es true — deberían ser distintos", i, j)
			}
		}
	}
}

func TestDiscoveryErrors_NotMatchNil(t *testing.T) {
	errs := []error{
		ErrDiscoveryDisabled,
		ErrNoCandidatesFound,
		ErrClarifyMaxRounds,
	}

	for _, err := range errs {
		if errors.Is(err, nil) {
			t.Errorf("errors.Is(%v, nil) es true — no debería", err)
		}
	}
}
