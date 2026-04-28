package domain

import (
	"time"

	"github.com/google/uuid"
)

// Modelo de dominio del usuario, alineado con la tabla users en SQL.

// UserStatus representa el estado de la cuenta
// Alineado con schema: ('active', 'inactive', 'suspended', 'pending_verification', 'locked')
type UserStatus string

const (
	StatusPendingVerification UserStatus = "pending_verification"
	StatusActive              UserStatus = "active"
	StatusInactive            UserStatus = "inactive"
	StatusSuspended           UserStatus = "suspended"
	StatusLocked              UserStatus = "locked"
)

// User representa un usuario del sistema (alineado con migration schema)
type User struct {
	ID                  uuid.UUID
	Email               string
	EmailVerified       bool
	EmailVerifiedAt     *time.Time `json:"email_verified_at,omitzero"`
	PasswordHash        string
	RoleID              uuid.UUID
	RoleName            string
	Status              UserStatus
	LoginCount          int
	FailedLoginAttempts int
	LastLoginAt         *time.Time `json:"last_login_at,omitzero"`
	LockedUntil         *time.Time `json:"locked_until,omitzero"`
	MFAEnabled          bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Métodos de dominio para manipulación del estado del usuario.

// NewUser crea un nuevo usuario con estado inicial pending_verification.
func NewUser(email, passwordHash string, roleID uuid.UUID) *User {
	now := time.Now()
	return &User{
		ID:                  uuid.Must(uuid.NewV7()),
		Email:               email,
		EmailVerified:       false,
		PasswordHash:        passwordHash,
		RoleID:              roleID,
		RoleName:            "client",
		Status:              StatusPendingVerification,
		LoginCount:          0,
		FailedLoginAttempts: 0,
		MFAEnabled:          false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

// VerifyEmail marca el email como verificado
func (u *User) VerifyEmail() {
	now := time.Now()
	u.EmailVerified = true
	u.EmailVerifiedAt = &now
	u.Status = StatusActive
	u.UpdatedAt = now
}

// RecordLogin registra un login exitoso
func (u *User) RecordLogin() {
	now := time.Now()
	u.LastLoginAt = &now
	u.LoginCount++
	u.FailedLoginAttempts = 0
	u.UpdatedAt = now
}

// RecordFailedLogin registra un intento de login fallido
func (u *User) RecordFailedLogin(maxAttempts int, lockDuration time.Duration) {
	u.FailedLoginAttempts++
	u.UpdatedAt = time.Now()

	// Bloquear si excedió intentos máximos
	if u.FailedLoginAttempts >= maxAttempts {
		lockedUntil := time.Now().Add(lockDuration)
		u.LockedUntil = &lockedUntil
		u.Status = StatusLocked
	}
}

// IsLocked verifica si la cuenta está bloqueada
func (u *User) IsLocked() bool {
	if u.LockedUntil == nil {
		return false
	}
	if u.Status == StatusLocked && time.Now().Before(*u.LockedUntil) {
		return true
	}
	// Desbloquear si pasó el tiempo
	if u.Status == StatusLocked && time.Now().After(*u.LockedUntil) {
		u.Status = StatusActive
		u.LockedUntil = nil
	}
	return false
}

// Unlock desbloquea la cuenta
func (u *User) Unlock() {
	u.Status = StatusActive
	u.LockedUntil = nil
	u.FailedLoginAttempts = 0
	u.UpdatedAt = time.Now()
}
