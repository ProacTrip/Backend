// Domain: Errores específicos del módulo user.
// Cada error incluye un código legible para mapeo HTTP RFC 9457.
package domain

import (
	"errors"
)

var (
	// Perfil
	ErrProfileNotFound         = errors.New("PROFILE_NOT_FOUND: perfil de usuario no encontrado")
	ErrMedicalProfileNotFound  = errors.New("MEDICAL_PROFILE_NOT_FOUND: perfil médico no encontrado")
	ErrTravelPrefsNotFound     = errors.New("TRAVEL_PREFS_NOT_FOUND: preferencias de viaje no encontradas")

	// Validación de campos
	ErrInvalidEnum            = errors.New("INVALID_ENUM: valor de enumeración inválido")
	ErrInvalidGender          = errors.New("INVALID_GENDER: género inválido")
	ErrInvalidBloodType       = errors.New("INVALID_BLOOD_TYPE: tipo de sangre inválido")
	ErrInvalidPreferredClass  = errors.New("INVALID_PREFERRED_CLASS: clase de cabina inválida")
	ErrInvalidSeatPreference  = errors.New("INVALID_SEAT_PREFERENCE: preferencia de asiento inválida")
	ErrInvalidCountryCode     = errors.New("INVALID_COUNTRY_CODE: código de país inválido")
	ErrInvalidLanguageCode    = errors.New("INVALID_LANGUAGE_CODE: código de idioma inválido")
	ErrInvalidCurrencyCode    = errors.New("INVALID_CURRENCY_CODE: código de moneda inválido")
	ErrInvalidTimezone        = errors.New("INVALID_TIMEZONE: zona horaria inválida")

	// Encriptación
	ErrEncryptionError = errors.New("ENCRYPTION_ERROR: fallo al encriptar datos")
	ErrDecryptionError = errors.New("DECRYPTION_ERROR: fallo al desencriptar datos")

	// Documentos
	ErrDocumentNotFound     = errors.New("DOCUMENT_NOT_FOUND: documento no encontrado")
	ErrInvalidDocumentType  = errors.New("INVALID_DOCUMENT_TYPE: tipo de documento inválido")
	ErrInvalidFileType      = errors.New("INVALID_FILE_TYPE: tipo de archivo no permitido")
	ErrFileTooLarge         = errors.New("FILE_TOO_LARGE: el archivo excede el tamaño máximo permitido")
	ErrDocumentNotReady     = errors.New("DOCUMENT_NOT_READY: el documento aún no está listo para procesar")
	ErrMaxDocumentsReached  = errors.New("MAX_DOCUMENTS_REACHED: se alcanzó el límite máximo de documentos")
	ErrDuplicateDocument    = errors.New("DUPLICATE_DOCUMENT: el documento ya fue subido previamente")
	ErrRateLimitExceeded    = errors.New("RATE_LIMIT_EXCEEDED: límite de subidas por minuto excedido")

	// Actualizaciones médicas pendientes
	ErrPendingUpdateNotFound = errors.New("PENDING_UPDATE_NOT_FOUND: actualización pendiente no encontrada")
	ErrPendingUpdateExpired  = errors.New("PENDING_UPDATE_EXPIRED: la actualización pendiente ha expirado")
	ErrInvalidPendingAction  = errors.New("INVALID_PENDING_ACTION: acción no válida para la actualización pendiente")

	// Avatares
	ErrInvalidMimeType = errors.New("INVALID_MIME_TYPE: tipo MIME no permitido")
	ErrAvatarNotFound  = errors.New("AVATAR_NOT_FOUND: archivo de avatar no encontrado en R2")

	// Teléfono y Layover
	ErrInvalidPhone      = errors.New("INVALID_PHONE: formato de teléfono inválido — se requiere E.164 (ej: +5491123456789)")
	ErrInvalidMaxLayover = errors.New("INVALID_MAX_LAYOVER: max_layover_duration debe ser ≥ 0")

	// Admin
	ErrPermissionDenied = errors.New("PERMISSION_DENIED: permiso denegado para esta acción administrativa")
)

