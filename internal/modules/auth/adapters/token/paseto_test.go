package token_test

import (
	"errors"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// testKey es una clave simétrica de 32 bytes exactos para PASETO V4.
var testKey = []byte("abcdefghijklmnopqrstuvwxyz123456")

// testUser contiene datos fijos de prueba reutilizados en todos los escenarios.
var testUser = struct {
	userID uuid.UUID
	email  string
	role   string
	roleID uuid.UUID
}{
	userID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
	email:  "test@proactrip.com",
	role:   "user",
	roleID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
}

// newTestService crea un PasetoService respaldado por una instancia fresca de miniredis.
// Usa RunT que inicia el servidor y registra cleanup automático.
func newTestService(t *testing.T) (*token.PasetoService, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	svc, err := token.NewPasetoService(token.PasetoConfig{
		SymmetricKey:    testKey,
		DragonflyClient: client,
	})
	if err != nil {
		t.Fatalf("NewPasetoService: %v", err)
	}

	return svc, mr
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 1: GenerateTokenPair — ambos tokens no vacíos, distintos,
// claims presentes (sub, email, role, type, jti, exp).
// ──────────────────────────────────────────────────────────────────────────────

func TestGenerateTokenPair_DevuelveAmbosTokensYContieneClaimsRequeridos(t *testing.T) {
	svc, _ := newTestService(t)

	pair, err := svc.GenerateTokenPair(
		testUser.userID,
		testUser.email,
		testUser.role,
		testUser.roleID,
	)
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	// Ambos tokens no vacíos.
	if pair.AccessToken == "" {
		t.Error("AccessToken está vacío")
	}
	if pair.RefreshToken == "" {
		t.Error("RefreshToken está vacío")
	}

	// Valores distintos.
	if pair.AccessToken == pair.RefreshToken {
		t.Error("AccessToken y RefreshToken son idénticos, deberían ser distintos")
	}

	// Verificar claims en el access token parseándolo directamente.
	parser := paseto.NewParser()
	parsed, err := parser.ParseV4Local(svc.SymmetricKey(), pair.AccessToken, nil)
	if err != nil {
		t.Fatalf("ParseV4Local access token: %v", err)
	}

	// 1. sub
	sub, err := parsed.GetSubject()
	if err != nil {
		t.Errorf("claim 'sub' ausente: %v", err)
	} else if sub == "" {
		t.Error("claim 'sub' está vacía")
	}

	// 2. email
	email, err := parsed.GetString("email")
	if err != nil {
		t.Errorf("claim 'email' ausente: %v", err)
	} else if email == "" {
		t.Error("claim 'email' está vacía")
	}

	// 3. role
	role, err := parsed.GetString("role")
	if err != nil {
		t.Errorf("claim 'role' ausente: %v", err)
	} else if role == "" {
		t.Error("claim 'role' está vacía")
	}

	// 4. type
	tokenType, err := parsed.GetString("type")
	if err != nil {
		t.Errorf("claim 'type' ausente: %v", err)
	} else if tokenType != "access" {
		t.Errorf("claim 'type' = %q, esperado 'access'", tokenType)
	}

	// 5. jti
	jti, err := parsed.GetJti()
	if err != nil {
		t.Errorf("claim 'jti' ausente: %v", err)
	} else if jti == "" {
		t.Error("claim 'jti' está vacía")
	}

	// 6. exp
	exp, err := parsed.GetExpiration()
	if err != nil {
		t.Errorf("claim 'exp' ausente: %v", err)
	} else if exp.IsZero() {
		t.Error("claim 'exp' es zero")
	}

	// Verificar también que el refresh token tiene type=refresh.
	parsedRefresh, err := parser.ParseV4Local(svc.SymmetricKey(), pair.RefreshToken, nil)
	if err != nil {
		t.Fatalf("ParseV4Local refresh token: %v", err)
	}
	refreshType, err := parsedRefresh.GetString("type")
	if err != nil {
		t.Errorf("refresh token claim 'type' ausente: %v", err)
	} else if refreshType != "refresh" {
		t.Errorf("refresh token claim 'type' = %q, esperado 'refresh'", refreshType)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 2: ValidateRefreshToken válido — extrae claims
// ──────────────────────────────────────────────────────────────────────────────

func TestValidateRefreshToken_Valido_ExtraeClaims(t *testing.T) {
	svc, _ := newTestService(t)

	refreshToken, err := svc.GenerateRefreshToken(
		testUser.userID,
		testUser.email,
		testUser.role,
		testUser.roleID,
	)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	claims, err := svc.ValidateRefreshToken(t.Context(), refreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}

	// sub → UserID
	if claims.UserID != testUser.userID {
		t.Errorf("sub/UserID = %s, esperado %s", claims.UserID, testUser.userID)
	}

	// email
	if claims.Email != testUser.email {
		t.Errorf("email = %q, esperado %q", claims.Email, testUser.email)
	}

	// role
	if claims.Role != testUser.role {
		t.Errorf("role = %q, esperado %q", claims.Role, testUser.role)
	}

	// jti
	if claims.JTI == uuid.Nil {
		t.Error("jti es UUID nil")
	}

	// type es verificado internamente por ValidateRefreshToken (debe ser "refresh").
	parser := paseto.NewParser()
	parsed, err := parser.ParseV4Local(svc.SymmetricKey(), refreshToken, nil)
	if err != nil {
		t.Fatalf("parseo adicional del refresh token: %v", err)
	}
	tokenType, err := parsed.GetString("type")
	if err != nil {
		t.Errorf("claim 'type' ausente en el token: %v", err)
	} else if tokenType != "refresh" {
		t.Errorf("claim 'type' = %q, esperado 'refresh'", tokenType)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 3: ValidateRefreshToken expirado — devuelve ErrTokenExpired.
// ──────────────────────────────────────────────────────────────────────────────

func TestValidateRefreshToken_Expirado_DevuelveErrTokenExpired(t *testing.T) {
	svc, _ := newTestService(t)

	// Construir un refresh token con expiración en el pasado.
	expiredToken := paseto.NewToken()
	expiredToken.SetSubject(testUser.userID.String())
	expiredToken.SetString("email", testUser.email)
	expiredToken.SetString("role", testUser.role)
	expiredToken.SetString("role_id", testUser.roleID.String())
	expiredToken.SetJti(uuid.New().String())
	expiredToken.SetString("type", "refresh")
	expiredToken.SetExpiration(time.Now().Add(-1 * time.Hour))

	expiredStr := expiredToken.V4Encrypt(svc.SymmetricKey(), nil)

	_, err := svc.ValidateRefreshToken(t.Context(), expiredStr)
	if err == nil {
		t.Fatal("se esperaba error para token expirado, se obtuvo nil")
	}

	if !errors.Is(err, domain.ErrTokenExpired) {
		t.Errorf("error = %v, se esperaba ErrTokenExpired", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 4: ValidateRefreshToken con tipo incorrecto — access token pasado
// como refresh devuelve error.
// ──────────────────────────────────────────────────────────────────────────────

func TestValidateRefreshToken_TipoIncorrecto_DevuelveError(t *testing.T) {
	svc, _ := newTestService(t)

	accessToken, err := svc.GenerateAccessToken(
		testUser.userID,
		testUser.email,
		testUser.role,
		testUser.roleID,
	)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	_, err = svc.ValidateRefreshToken(t.Context(), accessToken)
	if err == nil {
		t.Fatal("se esperaba error al validar access token como refresh, se obtuvo nil")
	}

	if !errors.Is(err, domain.ErrTokenInvalid) {
		t.Errorf("error = %v, se esperaba ErrTokenInvalid", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 5: isJTIBlacklisted fail-open — miniredis cerrado debe devolver
// (false, nil). Nunca error, nunca true.
// ──────────────────────────────────────────────────────────────────────────────

func TestIsJTIBlacklisted_FailOpen_DevuelveFalseNil(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis.Start: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	svc, err := token.NewPasetoService(token.PasetoConfig{
		SymmetricKey:    testKey,
		DragonflyClient: client,
	})
	if err != nil {
		t.Fatalf("NewPasetoService: %v", err)
	}

	mr.Close()

	refreshToken, genErr := svc.GenerateRefreshToken(
		testUser.userID,
		testUser.email,
		testUser.role,
		testUser.roleID,
	)
	if genErr != nil {
		t.Fatalf("GenerateRefreshToken: %v", genErr)
	}

	// With Dragonfly down, ValidateRefreshToken should succeed (fail-open)
	claims, valErr := svc.ValidateRefreshToken(t.Context(), refreshToken)
	if valErr != nil {
		t.Errorf("ValidateRefreshToken should fail-open when Dragonfly is down, got error: %v", valErr)
	}
	if claims == nil {
		t.Error("ValidateRefreshToken returned nil claims on fail-open")
	}
	if claims != nil && claims.UserID != testUser.userID {
		t.Errorf("claims.UserID = %s, expected %s", claims.UserID, testUser.userID)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 6: isJTIBlacklisted blacklisted — clave seteada en Dragonfly
// devuelve (true, nil).
// ──────────────────────────────────────────────────────────────────────────────

func TestIsJTIBlacklisted_Blacklisteada_DevuelveTrueNil(t *testing.T) {
	svc, mr := newTestService(t)

	jti := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	_ = jti

	// Generate a token pair and blacklist the refresh JTI.
	pair, err := svc.GenerateTokenPair(
		testUser.userID,
		testUser.email,
		testUser.role,
		testUser.roleID,
	)
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	// Blacklist the refresh JTI by setting it in miniredis.
	key := "{auth}:blacklist:jti:" + pair.RefreshJTI.String()
	mr.Set(key, "1")

	// ValidateRefreshToken should return ErrTokenRevoked for blacklisted JTI.
	_, err = svc.ValidateRefreshToken(t.Context(), pair.RefreshToken)
	if err == nil {
		t.Fatal("expected ErrTokenRevoked for blacklisted JTI, got nil")
	}
	if !errors.Is(err, domain.ErrTokenRevoked) {
		t.Errorf("error = %v, expected ErrTokenRevoked", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 7: isJTIBlacklisted no blacklisted — clave no seteada devuelve
// éxito.
// ──────────────────────────────────────────────────────────────────────────────

func TestIsJTIBlacklisted_NoBlacklisteada_DevuelveExito(t *testing.T) {
	svc, _ := newTestService(t)

	refreshToken, err := svc.GenerateRefreshToken(
		testUser.userID,
		testUser.email,
		testUser.role,
		testUser.roleID,
	)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	_, err = svc.ValidateRefreshToken(t.Context(), refreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: unexpected error: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 8: isExpiredTokenError — 3 sub-casos.
// ──────────────────────────────────────────────────────────────────────────────

func TestIsExpiredTokenError(t *testing.T) {
	svc, _ := newTestService(t)

	// Construir token expirado para obtener un RuleError real.
	expiredToken := paseto.NewToken()
	expiredToken.SetSubject(testUser.userID.String())
	expiredToken.SetString("type", "access")
	expiredToken.SetExpiration(time.Now().Add(-1 * time.Hour))
	expiredStr := expiredToken.V4Encrypt(svc.SymmetricKey(), nil)

	parser := paseto.NewParser()
	_, expiryErr := parser.ParseV4Local(svc.SymmetricKey(), expiredStr, nil)
	if expiryErr == nil {
		t.Fatal("se esperaba error del parser para token expirado, se obtuvo nil")
	}

	tests := []struct {
		nombre string
		err    error
		espera bool
	}{
		{
			nombre: "nil error → false",
			err:    nil,
			espera: false,
		},
		{
			nombre: "error de expiración real → true",
			err:    expiryErr,
			espera: true,
		},
		{
			nombre: "error no relacionado con expiración → false",
			err:    errors.New("otro tipo de error cualquiera"),
			espera: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			resultado := token.IsExpiredTokenError(tt.err)
			if resultado != tt.espera {
				t.Errorf("IsExpiredTokenError(%v) = %v, esperado %v", tt.err, resultado, tt.espera)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 9: GenerateSSEToken — type=sse en los claims.
// ──────────────────────────────────────────────────────────────────────────────

func TestGenerateSSEToken_ContieneTypeSSE(t *testing.T) {
	svc, _ := newTestService(t)

	sseToken, err := svc.GenerateSSEToken(testUser.userID, testUser.email)
	if err != nil {
		t.Fatalf("GenerateSSEToken: %v", err)
	}

	if sseToken == "" {
		t.Fatal("GenerateSSEToken devolvió token vacío")
	}

	// Validar con el parser que el claim type es "sse".
	parser := paseto.NewParser()
	parsed, err := parser.ParseV4Local(svc.SymmetricKey(), sseToken, nil)
	if err != nil {
		t.Fatalf("ParseV4Local sse token: %v", err)
	}

	tokenType, err := parsed.GetString("type")
	if err != nil {
		t.Errorf("claim 'type' ausente en SSE token: %v", err)
	} else if tokenType != "sse" {
		t.Errorf("claim 'type' = %q, esperado 'sse'", tokenType)
	}

	// Verificar subject.
	sub, err := parsed.GetSubject()
	if err != nil {
		t.Errorf("claim 'sub' ausente en SSE token: %v", err)
	} else if sub != testUser.userID.String() {
		t.Errorf("sub = %q, esperado %q", sub, testUser.userID.String())
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 10: GenerateOAuthStateToken — type=oauth_state, TTL apropiado.
// ──────────────────────────────────────────────────────────────────────────────

func TestGenerateOAuthStateToken_ContieneTypeOAuthStateYTTLApropiado(t *testing.T) {
	svc, _ := newTestService(t)

	stateToken, err := svc.GenerateOAuthStateToken()
	if err != nil {
		t.Fatalf("GenerateOAuthStateToken: %v", err)
	}

	if stateToken == "" {
		t.Fatal("GenerateOAuthStateToken devolvió token vacío")
	}

	// Validar con el parser que el claim type es "oauth_state".
	parser := paseto.NewParser()
	parsed, err := parser.ParseV4Local(svc.SymmetricKey(), stateToken, nil)
	if err != nil {
		t.Fatalf("ParseV4Local oauth state token: %v", err)
	}

	tokenType, err := parsed.GetString("type")
	if err != nil {
		t.Errorf("claim 'type' ausente en OAuth state token: %v", err)
	} else if tokenType != "oauth_state" {
		t.Errorf("claim 'type' = %q, esperado 'oauth_state'", tokenType)
	}

	// Verificar que el claim "state" existe y no está vacío.
	state, err := parsed.GetString("state")
	if err != nil {
		t.Errorf("claim 'state' ausente en OAuth state token: %v", err)
	} else if state == "" {
		t.Error("claim 'state' está vacío")
	}

	// Verificar que la expiración es ~10 minutos desde ahora.
	exp, err := parsed.GetExpiration()
	if err != nil {
		t.Errorf("claim 'exp' ausente: %v", err)
	} else {
		remaining := time.Until(exp)
		if remaining <= 0 {
			t.Error("el token OAuth state ya expiró")
		}
		if remaining > 11*time.Minute {
			t.Errorf("TTL demasiado largo: %v restante, se esperaba ~10min", remaining)
		}
	}

	// Verificación adicional: validar con ValidateOAuthStateToken.
	returnedState, err := svc.ValidateOAuthStateToken(stateToken)
	if err != nil {
		t.Fatalf("ValidateOAuthStateToken: %v", err)
	}
	if returnedState == "" {
		t.Error("ValidateOAuthStateToken devolvió state vacío")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Escenario 11: ValidateRefreshToken fail-open end-to-end — Dragonfly caído
// NO debe invalidar el refresh token.
// ──────────────────────────────────────────────────────────────────────────────

func TestValidateRefreshToken_FailOpen_DragonflyCaidoDevuelveClaims(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis.Start: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	svc, err := token.NewPasetoService(token.PasetoConfig{
		SymmetricKey:    testKey,
		DragonflyClient: client,
	})
	if err != nil {
		t.Fatalf("NewPasetoService: %v", err)
	}

	// Generate a valid refresh token while Dragonfly is up.
	refreshToken, genErr := svc.GenerateRefreshToken(
		testUser.userID,
		testUser.email,
		testUser.role,
		testUser.roleID,
	)
	if genErr != nil {
		t.Fatalf("GenerateRefreshToken: %v", genErr)
	}

	// Close miniredis to simulate Dragonfly crash.
	mr.Close()
	client.Close()

	// ValidateRefreshToken must return valid claims even with Dragonfly down (fail-open).
	claims, valErr := svc.ValidateRefreshToken(t.Context(), refreshToken)
	if valErr != nil {
		t.Fatalf("ValidateRefreshToken should fail-open when Dragonfly is down, got: %v", valErr)
	}
	if claims == nil {
		t.Fatal("ValidateRefreshToken returned nil claims during Dragonfly outage")
	}
	if claims.UserID != testUser.userID {
		t.Errorf("claims.UserID = %s, expected %s — fail-open must return correct claims",
			claims.UserID, testUser.userID)
	}
	if claims.Email != testUser.email {
		t.Errorf("claims.Email = %q, expected %q", claims.Email, testUser.email)
	}
	if claims.JTI == uuid.Nil {
		t.Error("claims.JTI should not be nil — fail-open must return complete claims")
	}
}
