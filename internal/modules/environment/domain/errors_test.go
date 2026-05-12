package domain

import (
	"errors"
	"testing"
)

func TestSentinelErrors_MessagesMatchSpec(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "ErrInvalidIP tiene mensaje en español",
			err:     ErrInvalidIP,
			wantMsg: "dirección IP inválida",
		},
		{
			name:    "ErrLocationProvider tiene mensaje en español",
			err:     ErrLocationProvider,
			wantMsg: "proveedor de ubicación no disponible",
		},
		{
			name:    "ErrRateLimitExceeded tiene mensaje en español",
			err:     ErrRateLimitExceeded,
			wantMsg: "límite de peticiones excedido",
		},
		{
			name:    "ErrInternal tiene mensaje en español",
			err:     ErrInternal,
			wantMsg: "error interno del servidor",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("error centinela es nil — no fue inicializado")
			}
			if tc.err.Error() != tc.wantMsg {
				t.Errorf("mensaje = %q, esperaba %q", tc.err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestSentinelErrors_WorkWithErrorsIs(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
		wrapped  error
		wantMatch bool
	}{
		{
			name:      "ErrInvalidIP detecta error envuelto",
			sentinel:  ErrInvalidIP,
			wrapped:   &validationError{msg: ErrInvalidIP.Error(), err: ErrInvalidIP},
			wantMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sentinel == nil {
				t.Fatal("error centinela es nil — no fue inicializado")
			}
			got := errors.Is(tc.wrapped, tc.sentinel)
			if got != tc.wantMatch {
				t.Errorf("errors.Is() = %v, esperaba %v", got, tc.wantMatch)
			}
		})
	}
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	errs := []error{ErrInvalidIP, ErrLocationProvider, ErrRateLimitExceeded, ErrInternal}
	for i, a := range errs {
		for j, b := range errs {
			if i != j && a.Error() == b.Error() {
				t.Errorf("error %q y error %q tienen el mismo mensaje — deben ser distintos", a, b)
			}
		}
	}
}

// validationError wraps a sentinel error, mimicking how usecases wrap domain errors.
type validationError struct {
	msg string
	err error
}

func (e *validationError) Error() string { return e.msg }
func (e *validationError) Unwrap() error { return e.err }
