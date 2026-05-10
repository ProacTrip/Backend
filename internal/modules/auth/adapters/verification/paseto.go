package verification

import (
	"context"
	"fmt"
	"time"

	"aidanwoods.dev/go-paseto/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Genera y valida tokens de verificación de email usando PASETO.
// Almacena JTI en Dragonfly para protección contra replay attacks.

type VerificationService struct {
	symmetricKey paseto.V4SymmetricKey
	ttl          time.Duration // Tiempo de validez del token (default: 24h)
	rdb          *redis.Client // Dragonfly client for JTI storage
}

// VerificationConfig configuración del servicio
type VerificationConfig struct {
	SymmetricKey []byte
	TTL          time.Duration
}

// NewVerificationService crea un nuevo servicio de verificación
func NewVerificationService(cfg VerificationConfig, rdb *redis.Client) (*VerificationService, error) {
	if len(cfg.SymmetricKey) != 32 {
		return nil, fmt.Errorf("symmetric key debe ser 32 bytes, tiene %d", len(cfg.SymmetricKey))
	}

	key, err := paseto.V4SymmetricKeyFromBytes(cfg.SymmetricKey)
	if err != nil {
		return nil, fmt.Errorf("error creando PASETO key: %w", err)
	}

	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 24 * time.Hour // Default: 24 horas
	}

	return &VerificationService{
		symmetricKey: key,
		ttl:          ttl,
		rdb:          rdb,
	}, nil
}

// =============================================================================
// Token Claims
// =============================================================================

// TokenClaims contiene los datos del token de verificación
type TokenClaims struct {
	Email string
	JTI   uuid.UUID
}

// =============================================================================
// Generación y validación
// =============================================================================

// GenerateToken genera un token de verificación para un email
func (s *VerificationService) GenerateToken(ctx context.Context, email string) (string, error) {
	jti := uuid.Must(uuid.NewV7())

	token := paseto.NewToken()
	token.SetSubject(email)
	token.SetString("email", email)
	token.SetString("jti", jti.String())
	token.SetString("type", "verification")
	token.SetExpiration(time.Now().Add(s.ttl))

	tokenString := token.V4Encrypt(s.symmetricKey, nil)

	// Store JTI in Dragonfly with TTL matching token TTL
	jtiKey := fmt.Sprintf("{auth}:verification_jti:%s", email)
	if err := s.rdb.Set(ctx, jtiKey, jti.String(), s.ttl).Err(); err != nil {
		// If we can't store in Dragonfly, we should fail securely
		return "", fmt.Errorf("error almacenando JTI en Dragonfly: %w", err)
	}

	return tokenString, nil
}

// VerifyToken valida un token de verificación y retorna el email
func (s *VerificationService) VerifyToken(ctx context.Context, tokenString string) (*TokenClaims, error) {
	parser := paseto.NewParser()

	token, err := parser.ParseV4Local(s.symmetricKey, tokenString, nil)
	if err != nil {
		return nil, fmt.Errorf("token inválido: %w", err)
	}

	// Verificar tipo
	tokenType, _ := token.GetString("type")
	if tokenType != "verification" {
		return nil, fmt.Errorf("token no es de verificación")
	}

	// Extraer email
	email, _ := token.GetSubject()
	if email == "" {
		return nil, fmt.Errorf("token sin email")
	}

	// Extraer JTI
	jtiStr, _ := token.GetString("jti")
	jti, _ := uuid.Parse(jtiStr)

	// Retrieve JTI from Dragonfly for validation
	jtiKey := fmt.Sprintf("{auth}:verification_jti:%s", email)
	storedJTIStr, err := s.rdb.Get(ctx, jtiKey).Result()
	if err != nil {
		if err == redis.Nil {
			// Token JTI not found in Dragonfly - token was already used or expired
			return nil, fmt.Errorf("token ya utilizado o expirado")
		}
		return nil, fmt.Errorf("error obteniendo JTI de Dragonfly: %w", err)
	}

	// Compare JTIs - if they don't match, token is invalid
	if storedJTIStr != jti.String() {
		return nil, fmt.Errorf("token JTI no coincide - posible replay attack")
	}

	return &TokenClaims{
		Email: email,
		JTI:   jti,
	}, nil
}
