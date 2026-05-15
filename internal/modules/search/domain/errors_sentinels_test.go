package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrBookingTokenExpiredExists(t *testing.T) {
	if ErrBookingTokenExpired == nil {
		t.Fatal("ErrBookingTokenExpired must be defined")
	}
	want := "BOOKING_TOKEN_EXPIRED: el token de reserva ha expirado o no es válido"
	if ErrBookingTokenExpired.Error() != want {
		t.Errorf("ErrBookingTokenExpired.Error() = %q, want %q",
			ErrBookingTokenExpired.Error(), want)
	}
}

func TestErrPropertyNotFoundExists(t *testing.T) {
	if ErrPropertyNotFound == nil {
		t.Fatal("ErrPropertyNotFound must be defined")
	}
	want := "PROPERTY_NOT_FOUND: la propiedad no fue encontrada"
	if ErrPropertyNotFound.Error() != want {
		t.Errorf("ErrPropertyNotFound.Error() = %q, want %q",
			ErrPropertyNotFound.Error(), want)
	}
}

func TestErrBookingTokenExpiredIsMatchable(t *testing.T) {
	err := fmt.Errorf("booking token expired: %w", ErrBookingTokenExpired)
	if !errors.Is(err, ErrBookingTokenExpired) {
		t.Error("errors.Is should match wrapped ErrBookingTokenExpired")
	}
}

func TestErrPropertyNotFoundIsMatchable(t *testing.T) {
	err := fmt.Errorf("property not found: %w", ErrPropertyNotFound)
	if !errors.Is(err, ErrPropertyNotFound) {
		t.Error("errors.Is should match wrapped ErrPropertyNotFound")
	}
}

func TestNewSentinelsDistinctFromExisting(t *testing.T) {
	if errors.Is(ErrBookingTokenExpired, ErrProviderUnavailable) {
		t.Error("ErrBookingTokenExpired should NOT match ErrProviderUnavailable")
	}
	if errors.Is(ErrPropertyNotFound, ErrNoResults) {
		t.Error("ErrPropertyNotFound should NOT match ErrNoResults")
	}
}
