// Tests de comportamiento para la entidad User del módulo auth.
// Verifica los 7 métodos de dominio: NewUser, VerifyEmail, RecordLogin,
// RecordFailedLogin, IsLocked, MaybeUnlock, y Unlock.
//
// Convenciones:
//   - Black-box testing (package domain_test).
//   - Table-driven con t.Run(), nombres de sub-tests en español.
//   - Solo stdlib testing, sin testify.
package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// T1.1 — TestNuevoUsuario: constructor NewUser
// =============================================================================

func TestNuevoUsuario(t *testing.T) {
	tests := []struct {
		nombre       string
		email        string
		passwordHash string
	}{
		{
			nombre:       "crear usuario con todos los campos inicializados correctamente",
			email:        "test@example.com",
			passwordHash: "hashed_abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			roleID := uuid.Must(uuid.NewV7())
			user := domain.NewUser(tt.email, tt.passwordHash, roleID)

			// ID debe ser UUIDv7 no nulo
			if user.ID == uuid.Nil {
				t.Error("ID no debería ser nil (se espera UUIDv7)")
			}

			// Email debe coincidir
			if user.Email != tt.email {
				t.Errorf("Email = %q, se esperaba %q", user.Email, tt.email)
			}

			// PasswordHash debe coincidir
			if user.PasswordHash != tt.passwordHash {
				t.Errorf("PasswordHash = %q, se esperaba %q", user.PasswordHash, tt.passwordHash)
			}

			// Status debe ser pending_verification
			if user.Status != domain.StatusPendingVerification {
				t.Errorf("Status = %s, se esperaba %s", user.Status, domain.StatusPendingVerification)
			}

			// RoleID debe coincidir
			if user.RoleID != roleID {
				t.Errorf("RoleID = %v, se esperaba %v", user.RoleID, roleID)
			}

			// RoleName por defecto es "client"
			if user.RoleName != "client" {
				t.Errorf("RoleName = %q, se esperaba %q", user.RoleName, "client")
			}

			// LoginCount debe ser 0
			if user.LoginCount != 0 {
				t.Errorf("LoginCount = %d, se esperaba 0", user.LoginCount)
			}

			// FailedLoginAttempts debe ser 0
			if user.FailedLoginAttempts != 0 {
				t.Errorf("FailedLoginAttempts = %d, se esperaba 0", user.FailedLoginAttempts)
			}

			// EmailVerified debe ser false
			if user.EmailVerified {
				t.Error("EmailVerified debería ser false")
			}

			// EmailVerifiedAt debe ser nil
			if user.EmailVerifiedAt != nil {
				t.Error("EmailVerifiedAt debería ser nil")
			}

			// LastLoginAt debe ser nil (nunca hizo login)
			if user.LastLoginAt != nil {
				t.Error("LastLoginAt debería ser nil para un usuario nuevo")
			}

			// LockedUntil debe ser nil (no está bloqueado)
			if user.LockedUntil != nil {
				t.Error("LockedUntil debería ser nil para un usuario nuevo")
			}

			// Timestamps no deben ser zero
			if user.CreatedAt.IsZero() {
				t.Error("CreatedAt no debería ser zero")
			}
			if user.UpdatedAt.IsZero() {
				t.Error("UpdatedAt no debería ser zero")
			}

			// CreatedAt y UpdatedAt deben ser iguales al momento de creación
			if !user.CreatedAt.Equal(user.UpdatedAt) {
				t.Error("CreatedAt y UpdatedAt deberían ser iguales en la creación")
			}
		})
	}
}

// =============================================================================
// T1.1 — TestVerificarEmail: método VerifyEmail
// =============================================================================

func TestVerificarEmail(t *testing.T) {
	tests := []struct {
		nombre          string
		estadoInicial   domain.UserStatus
		emailVerifiedIn bool
		estadoEsperado  domain.UserStatus
	}{
		{
			nombre:          "usuario pending_verification pasa a active",
			estadoInicial:   domain.StatusPendingVerification,
			emailVerifiedIn: false,
			estadoEsperado:  domain.StatusActive,
		},
		{
			nombre:          "usuario ya active se mantiene active",
			estadoInicial:   domain.StatusActive,
			emailVerifiedIn: true,
			estadoEsperado:  domain.StatusActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			user := crearUsuarioBase()
			user.Status = tt.estadoInicial
			user.EmailVerified = tt.emailVerifiedIn
			oldUpdatedAt := user.UpdatedAt

			// Pequeña pausa para asegurar que UpdatedAt cambia
			time.Sleep(1 * time.Millisecond)

			user.VerifyEmail()

			// Status debe ser el esperado
			if user.Status != tt.estadoEsperado {
				t.Errorf("Status = %s, se esperaba %s", user.Status, tt.estadoEsperado)
			}

			// EmailVerified debe ser true
			if !user.EmailVerified {
				t.Error("EmailVerified debería ser true después de VerifyEmail")
			}

			// EmailVerifiedAt debe estar asignado
			if user.EmailVerifiedAt == nil {
				t.Error("EmailVerifiedAt no debería ser nil después de VerifyEmail")
			}

			// UpdatedAt debe haberse actualizado
			if !user.UpdatedAt.After(oldUpdatedAt) {
				t.Error("UpdatedAt debería haberse actualizado")
			}
		})
	}
}

// =============================================================================
// T1.1 — TestRegistrarLogin: método RecordLogin
// =============================================================================

func TestRegistrarLogin(t *testing.T) {
	tests := []struct {
		nombre               string
		loginCountInicial    int
		failedAttemptsInicial int
		loginCountEsperado   int
	}{
		{
			nombre:               "primer login: LoginCount pasa de 0 a 1",
			loginCountInicial:    0,
			failedAttemptsInicial: 0,
			loginCountEsperado:   1,
		},
		{
			nombre:               "segundo login: LoginCount pasa de 1 a 2",
			loginCountInicial:    1,
			failedAttemptsInicial: 2, // tenía intentos fallidos previos
			loginCountEsperado:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			user := crearUsuarioBase()
			user.LoginCount = tt.loginCountInicial
			user.FailedLoginAttempts = tt.failedAttemptsInicial
			oldUpdatedAt := user.UpdatedAt

			time.Sleep(1 * time.Millisecond)

			user.RecordLogin()

			// LoginCount debe haberse incrementado en 1
			if user.LoginCount != tt.loginCountEsperado {
				t.Errorf("LoginCount = %d, se esperaba %d", user.LoginCount, tt.loginCountEsperado)
			}

			// FailedLoginAttempts debe resetearse a 0
			if user.FailedLoginAttempts != 0 {
				t.Errorf("FailedLoginAttempts = %d, se esperaba 0 (debe resetearse en login exitoso)",
					user.FailedLoginAttempts)
			}

			// LastLoginAt debe estar asignado
			if user.LastLoginAt == nil {
				t.Error("LastLoginAt no debería ser nil después de RecordLogin")
			}

			// UpdatedAt debe haberse actualizado
			if !user.UpdatedAt.After(oldUpdatedAt) {
				t.Error("UpdatedAt debería haberse actualizado")
			}
		})
	}
}

// =============================================================================
// T1.1 — TestRegistrarLoginFallido: método RecordFailedLogin
// =============================================================================

func TestRegistrarLoginFallido(t *testing.T) {
	tests := []struct {
		nombre              string
		maxAttempts         int
		llamadas            int
		esperaBloqueo       bool
		failedAttemptsFinal int
		descripcion         string
	}{
		{
			nombre:              "primer intento fallido: incrementa contador sin bloquear",
			maxAttempts:         5,
			llamadas:            1,
			esperaBloqueo:       false,
			failedAttemptsFinal: 1,
			descripcion:         "con maxAttempts=5, el primer fallo no bloquea",
		},
		{
			nombre:              "al alcanzar el máximo: bloquea la cuenta",
			maxAttempts:         3,
			llamadas:            3,
			esperaBloqueo:       true,
			failedAttemptsFinal: 3,
			descripcion:         "con maxAttempts=3, el tercer fallo bloquea la cuenta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			user := crearUsuarioBase()
			lockDuration := 15 * time.Minute

			for i := 0; i < tt.llamadas; i++ {
				time.Sleep(1 * time.Millisecond)
				user.RecordFailedLogin(tt.maxAttempts, lockDuration)
			}

			// FailedLoginAttempts debe coincidir
			if user.FailedLoginAttempts != tt.failedAttemptsFinal {
				t.Errorf("FailedLoginAttempts = %d, se esperaba %d",
					user.FailedLoginAttempts, tt.failedAttemptsFinal)
			}

			if tt.esperaBloqueo {
				// Status debe ser StatusLocked
				if user.Status != domain.StatusLocked {
					t.Errorf("Status = %s, se esperaba %s", user.Status, domain.StatusLocked)
				}

				// LockedUntil debe estar asignado
				if user.LockedUntil == nil {
					t.Error("LockedUntil no debería ser nil cuando la cuenta está bloqueada")
				}

				// LockedUntil debe estar en el futuro
				if !user.LockedUntil.After(time.Now()) {
					t.Error("LockedUntil debería estar en el futuro")
				}
			} else {
				// Status no debe ser StatusLocked
				if user.Status == domain.StatusLocked {
					t.Error("Status no debería ser StatusLocked estando por debajo del umbral")
				}

				// LockedUntil debe seguir nil
				if user.LockedUntil != nil {
					t.Error("LockedUntil debería ser nil cuando no se alcanzó el umbral")
				}
			}
		})
	}
}

// =============================================================================
// T1.1 — TestEstaBloqueado: método IsLocked (predicado puro)
// =============================================================================

func TestEstaBloqueado(t *testing.T) {
	tests := []struct {
		nombre      string
		lockedUntil *time.Time
		statusIn    domain.UserStatus
		resultado   bool
		descripcion string
	}{
		{
			nombre:      "LockedUntil nil: no está bloqueada",
			lockedUntil: nil,
			statusIn:    domain.StatusActive,
			resultado:   false,
			descripcion: "sin LockedUntil, IsLocked retorna false",
		},
		{
			nombre:      "LockedUntil futuro: cuenta bloqueada",
			lockedUntil: new(time.Now().Add(1 * time.Hour)),
			statusIn:    domain.StatusLocked,
			resultado:   true,
			descripcion: "LockedUntil en el futuro con Status=locked, retorna true",
		},
		{
			nombre:      "LockedUntil pasado: IsLocked retorna false (ya no está bloqueada)",
			lockedUntil: new(time.Now().Add(-1 * time.Hour)),
			statusIn:    domain.StatusLocked,
			resultado:   false,
			descripcion: "IsLocked es predicado puro: LockedUntil en pasado retorna false sin mutar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			user := crearUsuarioBase()
			user.Status = tt.statusIn
			user.LockedUntil = tt.lockedUntil

			oldStatus := user.Status
			oldLockedUntil := user.LockedUntil

			resultado := user.IsLocked()

			if resultado != tt.resultado {
				t.Errorf("IsLocked() = %v, se esperaba %v", resultado, tt.resultado)
			}

			// IsLocked es predicado PURO — nunca debe mutar el estado
			if user.Status != oldStatus {
				t.Errorf("IsLocked() mutó Status: era %s, ahora %s", oldStatus, user.Status)
			}
			if user.LockedUntil != oldLockedUntil {
				t.Error("IsLocked() mutó LockedUntil — debe ser predicado puro")
			}
		})
	}
}

// =============================================================================
// T1.1 — TestMaybeUnlock: método MaybeUnlock (desbloqueo por tiempo)
// =============================================================================

func TestMaybeUnlock(t *testing.T) {
	tests := []struct {
		nombre         string
		lockedUntil    *time.Time
		statusInicial  domain.UserStatus
		esperaCambio   bool
		descripcion    string
	}{
		{
			nombre:        "LockedUntil nil: no hace nada",
			lockedUntil:   nil,
			statusInicial: domain.StatusActive,
			esperaCambio:  false,
			descripcion:   "sin LockedUntil, MaybeUnlock no modifica nada",
		},
		{
			nombre:        "LockedUntil futuro: no desbloquea aún",
			lockedUntil:   new(time.Now().Add(1 * time.Hour)),
			statusInicial: domain.StatusLocked,
			esperaCambio:  false,
			descripcion:   "LockedUntil en futuro, MaybeUnlock no debe desbloquear",
		},
		{
			nombre:        "LockedUntil pasado: desbloquea automáticamente",
			lockedUntil:   new(time.Now().Add(-1 * time.Hour)),
			statusInicial: domain.StatusLocked,
			esperaCambio:  true,
			descripcion:   "LockedUntil en pasado, MaybeUnlock debe desbloquear",
		},
		{
			nombre:        "Status no locked: no hace nada aunque LockedUntil pasado",
			lockedUntil:   new(time.Now().Add(-1 * time.Hour)),
			statusInicial: domain.StatusActive,
			esperaCambio:  false,
			descripcion:   "Status no es Locked, MaybeUnlock no modifica",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			user := crearUsuarioBase()
			user.Status = tt.statusInicial
			user.LockedUntil = tt.lockedUntil

			user.MaybeUnlock()

			if tt.esperaCambio {
				if user.Status != domain.StatusActive {
					t.Errorf("Status después de MaybeUnlock = %s, se esperaba %s",
						user.Status, domain.StatusActive)
				}
				if user.LockedUntil != nil {
					t.Error("LockedUntil debería ser nil después de MaybeUnlock")
				}
			} else {
				if user.Status != tt.statusInicial {
					t.Errorf("Status mutó cuando no debía: era %s, ahora %s",
						tt.statusInicial, user.Status)
				}
			}
		})
	}
}

// =============================================================================
// T1.1 — TestDesbloquear: método Unlock
// =============================================================================

func TestDesbloquear(t *testing.T) {
	tests := []struct {
		nombre      string
		descripcion string
	}{
		{
			nombre:      "desbloquea una cuenta bloqueada reiniciando todos los contadores",
			descripcion: "Status→active, LockedUntil→nil, FailedLoginAttempts→0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			user := crearUsuarioBase()
			// Simular cuenta bloqueada con intentos fallidos acumulados
			future := time.Now().Add(30 * time.Minute)
			user.Status = domain.StatusLocked
			user.LockedUntil = &future
			user.FailedLoginAttempts = 5

			time.Sleep(1 * time.Millisecond)

			user.Unlock()

			// Status debe ser StatusActive
			if user.Status != domain.StatusActive {
				t.Errorf("Status = %s, se esperaba %s", user.Status, domain.StatusActive)
			}

			// LockedUntil debe ser nil
			if user.LockedUntil != nil {
				t.Error("LockedUntil debería ser nil después de Unlock")
			}

			// FailedLoginAttempts debe ser 0
			if user.FailedLoginAttempts != 0 {
				t.Errorf("FailedLoginAttempts = %d, se esperaba 0", user.FailedLoginAttempts)
			}

			// UpdatedAt debe haberse actualizado (Unlock llama a time.Now())
			if user.UpdatedAt.IsZero() {
				t.Error("UpdatedAt no debería ser zero después de Unlock")
			}
		})
	}
}

// =============================================================================
// Helpers
// =============================================================================

// crearUsuarioBase devuelve un User con estado pending_verification listo para testear.
func crearUsuarioBase() *domain.User {
	return domain.NewUser(
		"base@example.com",
		"hashed_base",
		uuid.Must(uuid.NewV7()),
	)
}
