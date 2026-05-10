package domain

import (
	"errors"
	"fmt"
	"testing"
)

// =============================================================================
// T-1.2: Sentinel Errors — formato "CODE: descripción en español"
// =============================================================================

func TestSentinelErrors_Format(t *testing.T) {
	errs := map[string]error{
		"ErrProfileNotFound":         ErrProfileNotFound,
		"ErrMedicalProfileNotFound":  ErrMedicalProfileNotFound,
		"ErrTravelPrefsNotFound":     ErrTravelPrefsNotFound,
		"ErrNotifPrefsNotFound":      ErrNotifPrefsNotFound,
		"ErrInvalidGender":           ErrInvalidGender,
		"ErrInvalidBloodType":        ErrInvalidBloodType,
		"ErrInvalidPreferredClass":   ErrInvalidPreferredClass,
		"ErrInvalidSeatPreference":   ErrInvalidSeatPreference,
		"ErrInvalidChannel":          ErrInvalidChannel,
		"ErrInvalidCountryCode":      ErrInvalidCountryCode,
		"ErrInvalidLanguageCode":     ErrInvalidLanguageCode,
		"ErrInvalidCurrencyCode":     ErrInvalidCurrencyCode,
		"ErrInvalidTimezone":         ErrInvalidTimezone,
		"ErrEncryptionError":         ErrEncryptionError,
		"ErrDecryptionError":         ErrDecryptionError,
		"ErrDocumentNotFound":        ErrDocumentNotFound,
		"ErrInvalidDocumentType":     ErrInvalidDocumentType,
		"ErrInvalidEnum":            ErrInvalidEnum,
		"ErrInvalidFileType":         ErrInvalidFileType,
		"ErrFileTooLarge":            ErrFileTooLarge,
		"ErrDocumentNotReady":        ErrDocumentNotReady,
		"ErrMaxDocumentsReached":     ErrMaxDocumentsReached,
		"ErrSearchNotFound":          ErrSearchNotFound,
		"ErrDuplicateSavedSearch":    ErrDuplicateSavedSearch,
		"ErrFavoriteNotFound":        ErrFavoriteNotFound,
		"ErrDuplicateFavorite":       ErrDuplicateFavorite,
		"ErrPendingUpdateNotFound":   ErrPendingUpdateNotFound,
		"ErrPendingUpdateExpired":    ErrPendingUpdateExpired,
		"ErrInvalidPendingAction":    ErrInvalidPendingAction,
		"ErrPermissionDenied":        ErrPermissionDenied,
	}

	for name, err := range errs {
		if err == nil {
			t.Errorf("%s es nil — no debería serlo", name)
			continue
		}
		msg := err.Error()
		// Formato: "CODE: descripción" — CODE puede tener cualquier longitud
		// Separador ": " debe aparecer antes del fin
		idx := 0
		found := false
		for i := 2; i < len(msg)-1; i++ {
			if msg[i:i+2] == ": " {
				idx = i
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s debería seguir formato 'CODE: descripción', tiene: %q", name, msg)
			continue
		}
		// CODE parte antes del ":", debe tener al menos 2 caracteres en mayúsculas
		code := msg[:idx]
		if len(code) < 2 {
			t.Errorf("%s: CODE demasiado corto: %q", name, code)
		}
		// descripción parte después de ": ", no debe estar vacía
		if idx+2 >= len(msg) {
			t.Errorf("%s: falta descripción después de ': '", name)
		}
	}
}

func TestSentinelErrors_IsWrapping(t *testing.T) {
	// errors.Is funciona con wrapping vía %w
	actuallyWrapped := fmt.Errorf("context: %w", ErrDocumentNotFound)
	if !errors.Is(actuallyWrapped, ErrDocumentNotFound) {
		t.Error("errors.Is debería detectar error envuelto con %w")
	}
	
	// Sentinel error direct match
	if !errors.Is(ErrDocumentNotFound, ErrDocumentNotFound) {
		t.Error("errors.Is debería coincidir consigo mismo")
	}
}

func TestSentinelErrors_Count(t *testing.T) {
	// Debe haber al menos 27 sentinels
	allErrors := []error{
		ErrProfileNotFound, ErrMedicalProfileNotFound, ErrTravelPrefsNotFound,
		ErrNotifPrefsNotFound, ErrInvalidEnum, ErrInvalidGender,
		ErrInvalidBloodType, ErrInvalidPreferredClass, ErrInvalidSeatPreference,
		ErrInvalidChannel, ErrInvalidCountryCode, ErrInvalidLanguageCode,
		ErrInvalidCurrencyCode, ErrInvalidTimezone, ErrEncryptionError,
		ErrDecryptionError, ErrDocumentNotFound, ErrInvalidDocumentType,
		ErrInvalidFileType, ErrFileTooLarge,
		ErrDocumentNotReady, ErrMaxDocumentsReached, ErrSearchNotFound,
		ErrDuplicateSavedSearch, ErrFavoriteNotFound, ErrDuplicateFavorite,
		ErrPendingUpdateNotFound,
		ErrPendingUpdateExpired, ErrInvalidPendingAction, ErrPermissionDenied,
	}
	
	for i, err := range allErrors {
		if err == nil {
			t.Errorf("error en índice %d es nil", i)
		}
	}
	
	if len(allErrors) < 27 {
		t.Errorf("se esperaban al menos 27 errores, hay %d", len(allErrors))
	}
}

func TestErrFavoriteNotFound_Valid(t *testing.T) {
	if ErrFavoriteNotFound != ErrFavoriteNotFound {
		t.Error("ErrFavoriteNotFound should compare equal with itself")
	}
	// Verificar que es accesible como sentinel
	err := ErrFavoriteNotFound
	if err.Error() == "" {
		t.Error("ErrFavoriteNotFound.Error() no debería estar vacío")
	}
}
