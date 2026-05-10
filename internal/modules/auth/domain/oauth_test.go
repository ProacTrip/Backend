// Test: domain OAuth entities — constructores y comportamiento.
// AuthIdentity es la única entidad con constructor (NewAuthIdentity).
// OAuthToken, OAuthUserInfo, OAuthState son DTOs simples sin comportamiento.
package domain_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Test: NewAuthIdentity crea identidad con campos correctos
// =============================================================================

func TestNewAuthIdentity_CamposCorrectos(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	identity := domain.NewAuthIdentity(
		userID,
		"google",
		"google-sub-123",
		"usuario@gmail.com",
		"Usuario Test",
		"https://example.com/avatar.jpg",
	)

	if identity.ID == uuid.Nil {
		t.Error("ID no debería ser nil UUID")
	}
	if identity.UserID != userID {
		t.Errorf("UserID: esperaba %v, obtuve %v", userID, identity.UserID)
	}
	if identity.ProviderCode != "google" {
		t.Errorf("ProviderCode: esperaba google, obtuve %s", identity.ProviderCode)
	}
	if identity.ProviderUserID != "google-sub-123" {
		t.Errorf("ProviderUserID: esperaba google-sub-123, obtuve %s", identity.ProviderUserID)
	}
	if identity.Email != "usuario@gmail.com" {
		t.Errorf("Email: esperaba usuario@gmail.com, obtuve %s", identity.Email)
	}
	if identity.DisplayName != "Usuario Test" {
		t.Errorf("DisplayName: esperaba Usuario Test, obtuve %s", identity.DisplayName)
	}
	if identity.AvatarURL != "https://example.com/avatar.jpg" {
		t.Errorf("AvatarURL: esperaba https://example.com/avatar.jpg, obtuve %s", identity.AvatarURL)
	}
	if identity.RawData == nil {
		t.Error("RawData no debería ser nil (se inicializa como map vacío)")
	}
	if identity.LastUsedAt.IsZero() {
		t.Error("LastUsedAt no debería ser zero")
	}
	if identity.CreatedAt.IsZero() {
		t.Error("CreatedAt no debería ser zero")
	}
	if identity.UpdatedAt.IsZero() {
		t.Error("UpdatedAt no debería ser zero")
	}
}

// =============================================================================
// Test: NewAuthIdentity genera IDs únicos (UUIDv7)
// =============================================================================

func TestNewAuthIdentity_IDsUnicos(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())

	id1 := domain.NewAuthIdentity(userID, "google", "sub-1", "a@b.com", "A", "").ID
	id2 := domain.NewAuthIdentity(userID, "google", "sub-2", "b@b.com", "B", "").ID

	if id1 == id2 {
		t.Error("dos NewAuthIdentity generaron el mismo ID")
	}
}

// =============================================================================
// Test: OAuthToken — construcción directa (DTO, sin constructor)
// =============================================================================

func TestOAuthToken_Construccion(t *testing.T) {
	token := domain.OAuthToken{
		AccessToken:  "ya29.test",
		RefreshToken: "1//refresh",
		ExpiresIn:    3600,
		TokenType:    "Bearer",
	}

	if token.AccessToken != "ya29.test" {
		t.Errorf("AccessToken: esperaba ya29.test, obtuve %s", token.AccessToken)
	}
	if token.TokenType != "Bearer" {
		t.Errorf("TokenType: esperaba Bearer, obtuve %s", token.TokenType)
	}
}

// =============================================================================
// Test: OAuthUserInfo — construcción directa (DTO, sin constructor)
// =============================================================================

func TestOAuthUserInfo_Construccion(t *testing.T) {
	info := domain.OAuthUserInfo{
		ProviderUserID: "1234567890",
		Email:          "usuario@gmail.com",
		EmailVerified:  true,
		Name:           "Usuario Test",
		Picture:        "https://lh3.googleusercontent.com/photo.jpg",
	}

	if !info.EmailVerified {
		t.Error("EmailVerified debería ser true")
	}
	if info.ProviderUserID != "1234567890" {
		t.Errorf("ProviderUserID: esperaba 1234567890, obtuve %s", info.ProviderUserID)
	}
}

// =============================================================================
// Test: OAuthState — construcción directa (DTO, sin constructor)
// =============================================================================

func TestOAuthState_Construccion(t *testing.T) {
	state := domain.OAuthState{
		CodeVerifier: "test-verifier-123",
	}

	if state.CodeVerifier != "test-verifier-123" {
		t.Errorf("CodeVerifier: esperaba test-verifier-123, obtuve %s", state.CodeVerifier)
	}
	if !state.CreatedAt.IsZero() {
		t.Log("CreatedAt es zero (se espera que el usecase lo establezca)")
	}
}

// =============================================================================
// Test: NewAuthIdentity con diferentes proveedores
// =============================================================================

func TestNewAuthIdentity_DistintosProveedores(t *testing.T) {
	tests := []struct {
		nombre     string
		provider   string
		providerID string
	}{
		{"google", "google", "g-123"},
		{"github", "github", "gh-456"},
		{"apple", "apple", "ap-789"},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			userID := uuid.Must(uuid.NewV7())
			identity := domain.NewAuthIdentity(userID, tt.provider, tt.providerID, "user@test.com", "User", "")

			if identity.ProviderCode != tt.provider {
				t.Errorf("ProviderCode: esperaba %s, obtuve %s", tt.provider, identity.ProviderCode)
			}
			if identity.ProviderUserID != tt.providerID {
				t.Errorf("ProviderUserID: esperaba %s, obtuve %s", tt.providerID, identity.ProviderUserID)
			}
		})
	}
}
