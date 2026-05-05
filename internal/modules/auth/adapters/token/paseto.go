package token

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aidanwoods.dev/go-paseto/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Servicio de tokens usando PASETO V4 Local.
// Genera y valida access/refresh tokens cifrados.

type PasetoConfig struct {
	SymmetricKey    []byte // 32 bytes exactos
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	DragonflyClient *redis.Client
}

type PasetoService struct {
	symmetricKey paseto.V4SymmetricKey
	accessTTL    time.Duration
	refreshTTL   time.Duration
	dragonfly    *redis.Client
}

func NewPasetoService(cfg PasetoConfig) (*PasetoService, error) {
	if len(cfg.SymmetricKey) != 32 {
		return nil, fmt.Errorf("la clave simétrica debe tener 32 bytes, tiene %d", len(cfg.SymmetricKey))
	}

	key, err := paseto.V4SymmetricKeyFromBytes(cfg.SymmetricKey)
	if err != nil {
		return nil, fmt.Errorf("error al crear clave PASETO: %w", err)
	}

	accessTTL := cfg.AccessTTL
	if accessTTL == 0 {
		accessTTL = 15 * time.Minute
	}
	refreshTTL := cfg.RefreshTTL
	if refreshTTL == 0 {
		refreshTTL = 7 * 24 * time.Hour
	}

	return &PasetoService{
		symmetricKey: key,
		accessTTL:    accessTTL,
		refreshTTL:   refreshTTL,
		dragonfly:    cfg.DragonflyClient,
	}, nil
}

func (s *PasetoService) GenerateTokenPair(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (*TokenPair, error) {
	accessToken, accessJTI, err := s.generateAccessToken(userID, email, roleID, sessionID)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshJTI, err := s.generateRefreshToken(userID, email, roleID, sessionID)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		AccessJTI:    accessJTI,
		RefreshJTI:   refreshJTI,
		AccessExp:    time.Now().Add(s.accessTTL),
		RefreshExp:   time.Now().Add(s.refreshTTL),
	}, nil
}

// GenerateAccessToken genera solo un access token.
func (s *PasetoService) GenerateAccessToken(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (string, error) {
	tokenStr, _, err := s.generateAccessToken(userID, email, roleID, sessionID)
	return tokenStr, err
}

func (s *PasetoService) GenerateRefreshToken(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (string, error) {
	tokenStr, _, err := s.generateRefreshToken(userID, email, roleID, sessionID)
	return tokenStr, err
}

// ValidateAccessToken valida un access token y devuelve sus claims.
func (s *PasetoService) ValidateAccessToken(ctx context.Context, tokenString string) (*AccessClaims, error) {
	parser := paseto.NewParser() // comprueba expiración automáticamente

	pasetoToken, err := parser.ParseV4Local(s.symmetricKey, tokenString, nil)
	if err != nil {
		return nil, errors.Join(domain.ErrTokenInvalid, err)
	}

	// Verificar tipo de token
	tokenType, err := pasetoToken.GetString("type")
	if err != nil || tokenType != "access" {
		return nil, domain.ErrTokenInvalid
	}

	// Extraer claims obligatorios
	subject, err := pasetoToken.GetSubject()
	if err != nil || subject == "" {
		return nil, domain.ErrTokenInvalid
	}
	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	email, err := pasetoToken.GetString("email")
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	roleIDStr, err := pasetoToken.GetString("role_id")
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	sessionIDStr, err := pasetoToken.GetString("session_id")
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	jti, err := pasetoToken.GetJti()
	if err != nil || jti == "" {
		return nil, domain.ErrTokenInvalid
	}
	jtiUUID, err := uuid.Parse(jti)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	return &AccessClaims{
		UserID:    userID,
		Email:     email,
		RoleID:    roleID,
		SessionID: sessionID,
		JTI:       jtiUUID,
	}, nil
}

func (s *PasetoService) ValidateRefreshToken(ctx context.Context, tokenString string) (*RefreshClaims, error) {
	parser := paseto.NewParser()

	pasetoToken, err := parser.ParseV4Local(s.symmetricKey, tokenString, nil)
	if err != nil {
		if isExpiredTokenError(err) {
			return nil, domain.ErrTokenExpired
		}
		return nil, errors.Join(domain.ErrTokenInvalid, err)
	}

	tokenType, err := pasetoToken.GetString("type")
	if err != nil || tokenType != "refresh" {
		return nil, domain.ErrTokenInvalid
	}

	subject, err := pasetoToken.GetSubject()
	if err != nil || subject == "" {
		return nil, domain.ErrTokenInvalid
	}
	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	email, _ := pasetoToken.GetString("email")

	roleIDStr, _ := pasetoToken.GetString("role_id")
	roleID, _ := uuid.Parse(roleIDStr)

	sessionIDStr, err := pasetoToken.GetString("session_id")
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	jti, err := pasetoToken.GetJti()
	if err != nil || jti == "" {
		return nil, domain.ErrTokenInvalid
	}
	jtiUUID, err := uuid.Parse(jti)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	claims := &RefreshClaims{
		UserID:    userID,
		Email:     email,
		RoleID:    roleID,
		SessionID: sessionID,
		JTI:       jtiUUID,
	}

	if blacklisted, err := s.isJTIBlacklisted(ctx, jtiUUID); err != nil || blacklisted {
		return nil, domain.ErrTokenRevoked
	}

	return claims, nil
}

// ValidateAndRotateRefresh valida el refresh token, blacklistea el JTI anterior
// y genera un nuevo par de tokens (access + refresh). Implementa rotación silenciosa.
func (s *PasetoService) ValidateAndRotateRefresh(ctx context.Context, refreshToken string) (*RefreshClaims, string, string, error) {
	claims, err := s.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, "", "", err
	}

	if err := s.blacklistJTI(ctx, claims.JTI); err != nil {
		return nil, "", "", fmt.Errorf("blacklist JTI: %w", err)
	}

	newAccess, err := s.GenerateAccessToken(claims.UserID, claims.Email, claims.RoleID, claims.SessionID)
	if err != nil {
		return nil, "", "", fmt.Errorf("generate access token: %w", err)
	}

	newRefresh, err := s.GenerateRefreshToken(claims.UserID, claims.Email, claims.RoleID, claims.SessionID)
	if err != nil {
		return nil, "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	return claims, newAccess, newRefresh, nil
}

func (s *PasetoService) blacklistJTI(ctx context.Context, jti uuid.UUID) error {
	if s.dragonfly == nil {
		return nil
	}
	key := fmt.Sprintf("auth:blacklist:jti:%s", jti.String())
	return s.dragonfly.Set(ctx, key, "1", s.refreshTTL).Err()
}

func (s *PasetoService) isJTIBlacklisted(ctx context.Context, jti uuid.UUID) (bool, error) {
	if s.dragonfly == nil {
		return false, nil
	}
	key := fmt.Sprintf("auth:blacklist:jti:%s", jti.String())
	result, err := s.dragonfly.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// ---------------------------------------------------------------------------
// Métodos privados de generación
// ---------------------------------------------------------------------------

func (s *PasetoService) generateAccessToken(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (string, uuid.UUID, error) {
	jti := uuid.Must(uuid.NewV7())

	token := paseto.NewToken()
	token.SetSubject(userID.String())
	token.SetString("email", email)
	token.SetString("role_id", roleID.String())
	token.SetString("session_id", sessionID.String())
	token.SetJti(jti.String())
	token.SetString("type", "access")
	token.SetExpiration(time.Now().Add(s.accessTTL))

	encrypted := token.V4Encrypt(s.symmetricKey, nil)
	return encrypted, jti, nil
}

func (s *PasetoService) generateRefreshToken(userID uuid.UUID, email string, roleID, sessionID uuid.UUID) (string, uuid.UUID, error) {
	jti := uuid.Must(uuid.NewV7())

	token := paseto.NewToken()
	token.SetSubject(userID.String())
	token.SetString("email", email)
	token.SetString("role_id", roleID.String())
	token.SetString("session_id", sessionID.String())
	token.SetJti(jti.String())
	token.SetString("type", "refresh")
	token.SetExpiration(time.Now().Add(s.refreshTTL))

	encrypted := token.V4Encrypt(s.symmetricKey, nil)
	return encrypted, jti, nil
}

// ---------------------------------------------------------------------------
// Tipos de retorno (pueden moverse a otro archivo si lo deseas)
// ---------------------------------------------------------------------------

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	AccessJTI    uuid.UUID
	RefreshJTI   uuid.UUID
	AccessExp    time.Time
	RefreshExp   time.Time
}

type AccessClaims struct {
	UserID    uuid.UUID
	Email     string
	RoleID    uuid.UUID
	SessionID uuid.UUID
	JTI       uuid.UUID
}

// GetUserID returns the user ID as a UUID for interface satisfaction.
func (c AccessClaims) GetUserID() uuid.UUID { return c.UserID }

type RefreshClaims struct {
	UserID    uuid.UUID
	Email     string
	RoleID    uuid.UUID
	SessionID uuid.UUID
	JTI       uuid.UUID
}

// GetUserID returns the user ID as a UUID for interface satisfaction.
func (c RefreshClaims) GetUserID() uuid.UUID { return c.UserID }

// =============================================================================
// SSE / OAuth / MFA Token Methods
// =============================================================================

func (s *PasetoService) GenerateSSEToken(userID, email string) (string, error) {
	token := paseto.NewToken()
	token.SetSubject(userID)
	token.SetString("email", email)
	token.SetString("type", "sse")
	token.SetExpiration(time.Now().Add(30 * time.Second))

	return token.V4Encrypt(s.symmetricKey, nil), nil
}

func (s *PasetoService) GenerateOAuthStateToken() (string, error) {
	state := uuid.Must(uuid.NewV7()).String()

	token := paseto.NewToken()
	token.SetString("state", state)
	token.SetString("type", "oauth_state")
	token.SetExpiration(time.Now().Add(10 * time.Minute))

	return token.V4Encrypt(s.symmetricKey, nil), nil
}

func (s *PasetoService) GenerateMFASessionToken(userID, email string) (string, error) {
	jti := uuid.Must(uuid.NewV7())

	token := paseto.NewToken()
	token.SetSubject(userID)
	token.SetString("email", email)
	token.SetJti(jti.String())
	token.SetString("type", "mfa_session")
	token.SetExpiration(time.Now().Add(5 * time.Minute))

	return token.V4Encrypt(s.symmetricKey, nil), nil
}

func (s *PasetoService) ValidateSSEToken(tokenString string) (*SSEClaims, error) {
	parser := paseto.NewParser()

	pasetoToken, err := parser.ParseV4Local(s.symmetricKey, tokenString, nil)
	if err != nil {
		if isExpiredTokenError(err) {
			return nil, domain.ErrTokenExpired
		}
		return nil, errors.Join(domain.ErrTokenInvalid, err)
	}

	tokenType, err := pasetoToken.GetString("type")
	if err != nil || tokenType != "sse" {
		return nil, domain.ErrTokenInvalid
	}

	subject, err := pasetoToken.GetSubject()
	if err != nil || subject == "" {
		return nil, domain.ErrTokenInvalid
	}
	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	email, err := pasetoToken.GetString("email")
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	return &SSEClaims{UserID: userID, Email: email}, nil
}

func (s *PasetoService) ValidateOAuthStateToken(tokenString string) (string, error) {
	parser := paseto.NewParser()

	pasetoToken, err := parser.ParseV4Local(s.symmetricKey, tokenString, nil)
	if err != nil {
		return "", errors.Join(domain.ErrTokenInvalid, err)
	}

	tokenType, err := pasetoToken.GetString("type")
	if err != nil || tokenType != "oauth_state" {
		return "", domain.ErrTokenInvalid
	}

	state, err := pasetoToken.GetString("state")
	if err != nil {
		return "", domain.ErrTokenInvalid
	}

	return state, nil
}

func (s *PasetoService) ValidateMFASessionToken(tokenString string) (*MFAClaims, error) {
	parser := paseto.NewParser()

	pasetoToken, err := parser.ParseV4Local(s.symmetricKey, tokenString, nil)
	if err != nil {
		if isExpiredTokenError(err) {
			return nil, domain.ErrTokenExpired
		}
		return nil, errors.Join(domain.ErrTokenInvalid, err)
	}

	tokenType, err := pasetoToken.GetString("type")
	if err != nil || tokenType != "mfa_session" {
		return nil, domain.ErrTokenInvalid
	}

	subject, err := pasetoToken.GetSubject()
	if err != nil || subject == "" {
		return nil, domain.ErrTokenInvalid
	}
	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	email, err := pasetoToken.GetString("email")
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	jti, err := pasetoToken.GetJti()
	if err != nil || jti == "" {
		return nil, domain.ErrTokenInvalid
	}
	jtiUUID, err := uuid.Parse(jti)
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}

	return &MFAClaims{UserID: userID, Email: email, JTI: jtiUUID}, nil
}

// =============================================================================
// Helpers
// =============================================================================

func isExpiredTokenError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "EXPIRED")
}

// =============================================================================
// Token Claim Types
// =============================================================================

type SSEClaims struct {
	UserID uuid.UUID
	Email  string
}

type OAuthStateClaims struct {
	State string
}

type MFAClaims struct {
	UserID uuid.UUID
	Email  string
	JTI    uuid.UUID
}
