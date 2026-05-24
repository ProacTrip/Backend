package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// OAuthProvider — interfaz para proveedores de autenticación externa (Google, etc.)
// Cada proveedor implementa la construcción de URL de autorización,
// intercambio de código por tokens y obtención de datos del usuario.
// ---------------------------------------------------------------------------

// OAuthProvider es la interfaz que debe implementar cada proveedor OAuth.
type OAuthProvider interface {
	// GetAuthURL construye la URL de autorización del proveedor
	// con los parámetros necesarios: client_id, redirect_uri, scope, state, PKCE.
	GetAuthURL(state, codeChallenge string) string

	// ExchangeCode intercambia un código de autorización por un token OAuth.
	ExchangeCode(ctx context.Context, code, codeVerifier string) (*OAuthToken, error)

	// GetUserInfo obtiene la información del usuario autenticado del proveedor.
	GetUserInfo(ctx context.Context, accessToken string) (*OAuthUserInfo, error)
}

// ---------------------------------------------------------------------------
// DTOs del dominio OAuth
// ---------------------------------------------------------------------------

// OAuthToken representa los tokens devueltos por el proveedor OAuth.
type OAuthToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	TokenType    string
}

// OAuthUserInfo representa los datos del usuario devueltos por el proveedor.
type OAuthUserInfo struct {
	ProviderUserID string // sub en Google, id en otros proveedores
	Email          string
	EmailVerified  bool
	Name           string // nombre completo (campo "name" de Google)
	GivenName      string // nombre de pila (campo "given_name" de Google), puede estar vacío
	FamilyName     string // apellido (campo "family_name" de Google), puede estar vacío
	Locale         string // locale del usuario (campo "locale" de Google), puede estar vacío
	Picture        string
}

// OAuthState representa el estado de la solicitud OAuth almacenado en caché.
type OAuthState struct {
	CodeVerifier string    // PKCE code_verifier generado
	CreatedAt    time.Time // momento de creación para auditoría
}

// ---------------------------------------------------------------------------
// AuthIdentity — entidad de dominio para identidades de autenticación externas
// Alineada con la tabla user_auth_identities.
// ---------------------------------------------------------------------------

type AuthIdentity struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	ProviderCode    string // ej. "google"
	ProviderUserID  string // sub/id del proveedor
	Email           string
	DisplayName     string
	AvatarURL       string
	AccessTokenEnc  string
	RefreshTokenEnc string
	TokenExpiresAt  *time.Time `json:"token_expires_at,omitzero"`
	RawData         map[string]any
	LastUsedAt      time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewAuthIdentity crea una nueva identidad de autenticación con ID UUIDv7.
func NewAuthIdentity(userID uuid.UUID, providerCode, providerUserID, email, displayName, avatarURL string) *AuthIdentity {
	now := time.Now()
	return &AuthIdentity{
		ID:             uuid.Must(uuid.NewV7()),
		UserID:         userID,
		ProviderCode:   providerCode,
		ProviderUserID: providerUserID,
		Email:          email,
		DisplayName:    displayName,
		AvatarURL:      avatarURL,
		RawData:        make(map[string]any),
		LastUsedAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
