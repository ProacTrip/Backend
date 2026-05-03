package authorize

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// Lógica de negocio de la autorización OAuth.
// Flujo: validar proveedor → generar state (PASETO) + PKCE → cachear en Dragonfly → construir auth URL.

// OAuthStateTokenService genera y valida tokens de estado OAuth (PASETO).
type OAuthStateTokenService interface {
	GenerateOAuthStateToken() (string, error)
	ValidateOAuthStateToken(tokenString string) (string, error)
}

// OAuthProviderSelector resuelve el proveedor OAuth por código (ej. "google").
type OAuthProviderSelector interface {
	GetProvider(code string) (domain.OAuthProvider, error)
}

// UseCase — lógica de negocio de la autorización OAuth.
type UseCase struct {
	stateTokenSvc OAuthStateTokenService
	providerSel   OAuthProviderSelector
	dragonfly     *redis.Client
}

type UseCaseDeps struct {
	StateTokenSvc OAuthStateTokenService
	ProviderSel   OAuthProviderSelector
	Dragonfly     *redis.Client
}

func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		stateTokenSvc: deps.StateTokenSvc,
		providerSel:   deps.ProviderSel,
		dragonfly:     deps.Dragonfly,
	}
}

// Execute genera la URL de autorización OAuth para el proveedor especificado.
// 1. Valida que el proveedor exista.
// 2. Genera state token (PASETO anti-CSRF).
// 3. Genera PKCE code_verifier (32 bytes random, base64url) + code_challenge (SHA256).
// 4. Cachea el code_verifier en Dragonfly con TTL de 10 minutos.
// 5. Construye y retorna la URL de autorización.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Obtener el proveedor OAuth
	provider, err := uc.providerSel.GetProvider(cmd.Provider)
	if err != nil {
		return nil, err
	}

	// 2. Generar state token PASETO (anti-CSRF + signed + expirable)
	stateToken, err := uc.stateTokenSvc.GenerateOAuthStateToken()
	if err != nil {
		return nil, fmt.Errorf("generar state token OAuth: %w", err)
	}

	// 3. Extraer el state value interno del PASETO para usarlo como key de Dragonfly
	stateValue, err := uc.stateTokenSvc.ValidateOAuthStateToken(stateToken)
	if err != nil {
		return nil, fmt.Errorf("validar state token generado: %w", err)
	}

	// 4. Generar PKCE code_verifier (32 bytes aleatorios, base64url sin padding)
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generar PKCE code_verifier: %w", err)
	}

	// 5. Calcular code_challenge = SHA256(code_verifier) en base64url sin padding
	codeChallenge := generateCodeChallenge(codeVerifier)

	// 6. Cachear state en Dragonfly: key="oauth:state:{state_value}", TTL=10min
	// El valor se guarda como JSON: { "code_verifier": "...", "created_at": "..." }
	oauthState := domain.OAuthState{
		CodeVerifier: codeVerifier,
		CreatedAt:    time.Now(),
	}
	stateJSON, err := json.Marshal(oauthState)
	if err != nil {
		return nil, fmt.Errorf("serializar estado OAuth: %w", err)
	}

	cacheKey := fmt.Sprintf("oauth:state:%s", stateValue)
	if err := uc.dragonfly.Set(ctx, cacheKey, string(stateJSON), 10*time.Minute).Err(); err != nil {
		return nil, fmt.Errorf("cachear estado OAuth en Dragonfly: %w", err)
	}

	// 7. Construir URL de autorización del proveedor
	authURL := provider.GetAuthURL(stateToken, codeChallenge)

	return &Response{
		AuthURL: authURL,
	}, nil
}

// generateCodeVerifier genera un PKCE code_verifier de 32 bytes aleatorios en base64url sin padding.
func generateCodeVerifier() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("leer bytes aleatorios: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// generateCodeChallenge calcula SHA256(codeVerifier) y lo codifica en base64url sin padding.
func generateCodeChallenge(codeVerifier string) string {
	h := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
