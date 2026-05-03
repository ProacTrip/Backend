package domain

import (
	"errors"
)

// Errores de dominio específicos del módulo auth.
// Cada error incluye un código legible para mapeo HTTP RFC 7807.
var (
	// Usuario
	ErrUserNotFound        = errors.New("USER_NOT_FOUND: usuario no encontrado")
	ErrEmailAlreadyExists  = errors.New("EMAIL_ALREADY_EXISTS: el email ya está registrado")
	ErrUserAlreadyVerified = errors.New("USER_ALREADY_VERIFIED: el usuario ya está verificado")
	ErrAccountLocked       = errors.New("ACCOUNT_LOCKED: cuenta bloqueada por intentos fallidos")
	ErrAccountSuspended    = errors.New("ACCOUNT_SUSPENDED: cuenta suspendida")
	ErrAccountInactive     = errors.New("ACCOUNT_INACTIVE: cuenta inactiva")
	ErrAccountDisabled     = errors.New("ACCOUNT_DISABLED: cuenta deshabilitada")
	ErrEmailNotVerified    = errors.New("EMAIL_NOT_VERIFIED: email no verificado")
	ErrAccountPending      = errors.New("ACCOUNT_PENDING: cuenta pendiente de verificación")

	// Credenciales y autenticación
	ErrInvalidCredentials = errors.New("INVALID_CREDENTIALS: credenciales inválidas")
	ErrWeakPassword       = errors.New("WEAK_PASSWORD: la contraseña es demasiado débil")
	ErrPasswordMismatch   = errors.New("PASSWORD_MISMATCH: las contraseñas no coinciden")
	ErrInvalidPassword    = errors.New("INVALID_PASSWORD: formato de contraseña inválido")
	ErrPasswordTooShort   = errors.New("PASSWORD_TOO_SHORT: la contraseña debe tener al menos 8 caracteres")

	// Autenticación
	ErrNotAuthenticated = errors.New("NOT_AUTHENTICATED: se requiere autenticación")

	// Tokens
	ErrTokenExpired             = errors.New("TOKEN_EXPIRED: token expirado")
	ErrTokenInvalid             = errors.New("TOKEN_INVALID: token inválido")
	ErrTokenRevoked             = errors.New("TOKEN_REVOKED: token revocado")
	ErrTokenNotFound            = errors.New("TOKEN_NOT_FOUND: token no encontrado")
	ErrInvalidVerificationToken = errors.New("INVALID_VERIFICATION_TOKEN: token de verificación inválido o expirado")
	ErrSessionExpired           = errors.New("SESSION_EXPIRED: sesión expirada")
	ErrSessionNotFound          = errors.New("SESSION_NOT_FOUND: sesión no encontrada")

	// OAuth
	ErrOAuthProviderNotFound = errors.New("OAUTH_PROVIDER_NOT_FOUND: proveedor OAuth no soportado")
	ErrOAuthCodeMissing      = errors.New("OAUTH_CODE_MISSING: el parámetro code no fue proporcionado por el proveedor")
	ErrOAuthStateMissing     = errors.New("OAUTH_STATE_MISSING: el parámetro state no fue proporcionado por el proveedor")
	ErrOAuthStateInvalid     = errors.New("OAUTH_STATE_INVALID: state inválido o expirado (posible CSRF)")
	ErrOAuthAccessDenied     = errors.New("OAUTH_ACCESS_DENIED: el usuario denegó el acceso o hubo un error del proveedor")
	ErrOAuthExchangeFailed   = errors.New("OAUTH_EXCHANGE_FAILED: fallo al intercambiar code por token con el proveedor")

	// Identidad
	ErrIdentityNotFound      = errors.New("IDENTITY_NOT_FOUND: identidad externa no encontrada")
	ErrIdentityAlreadyExists = errors.New("IDENTITY_ALREADY_EXISTS: ya existe una identidad vinculada a este proveedor")

	// MFA
	ErrMFARequired               = errors.New("MFA_REQUIRED: se requiere autenticación multifactor")
	ErrMFAInvalidCode            = errors.New("MFA_INVALID_CODE: código MFA inválido")
	ErrMFANotEnabled             = errors.New("MFA_NOT_ENABLED: MFA no habilitado para este usuario")
	ErrMFAAlreadyEnabled         = errors.New("MFA_ALREADY_ENABLED: MFA ya está habilitado")
	ErrMFAInvalidMethod          = errors.New("MFA_INVALID_METHOD: método MFA no configurado para este usuario")
	ErrMFARequiredCode           = errors.New("MFA_REQUIRED_CODE: código es requerido para este método")
	ErrMFACodeExpired            = errors.New("MFA_CODE_EXPIRED: código MFA expirado o inválido")
	ErrInvalidBackupCode         = errors.New("INVALID_BACKUP_CODE: código de respaldo inválido")
	ErrMFAInvalidRecoveryCode    = errors.New("MFA_INVALID_RECOVERY_CODE: código de recuperación inválido")
	ErrMFARecoveryCodesExhausted = errors.New("MFA_RECOVERY_CODES_EXHAUSTED: códigos de recuperación agotados")

	// Validación
	ErrInvalidEmail    = errors.New("INVALID_EMAIL: email con formato inválido")
	ErrInvalidInput    = errors.New("INVALID_INPUT: datos de entrada inválidos")
	ErrValidationError = errors.New("VALIDATION_ERROR: error de validación")

	// Roles y permisos
	ErrRoleNotFound               = errors.New("ROLE_NOT_FOUND: rol no encontrado")
	ErrPermissionNotFound         = errors.New("PERMISSION_NOT_FOUND: permiso no encontrado")
	ErrPermissionDenied           = errors.New("PERMISSION_DENIED: permiso denegado")
	ErrFeatureLimitNotFound       = errors.New("FEATURE_LIMIT_NOT_FOUND: límite de feature no encontrado")
	ErrInvalidBlockDuration       = errors.New("INVALID_BLOCK_DURATION: duración del bloqueo inválida")
	ErrInvalidReason              = errors.New("INVALID_REASON: razón requerida para la acción")
	ErrPermissionOverrideNotFound = errors.New("PERMISSION_OVERRIDE_NOT_FOUND: override de permiso no encontrado")
)
