// Test: GoogleOAuth — constructor, GetAuthURL, ExchangeCode (éxito/error), GetUserInfo (éxito/error).
// Usa httptest + RoundTripper custom para simular los endpoints de Google OAuth.
package oauth_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/config"
	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/oauth"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// =============================================================================
// Constructor
// =============================================================================

func TestNewGoogleOAuth_NoNil(t *testing.T) {
	cfg := config.OAuthConfig{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		GoogleRedirectURL:  "http://localhost:8080/v1/auth/oauth/google/callback",
	}
	provider := oauth.NewGoogleOAuth(cfg)
	if provider == nil {
		t.Fatal("NewGoogleOAuth retornó nil")
	}
}

// =============================================================================
// Compile-time: GoogleOAuth implementa domain.OAuthProvider
// =============================================================================

func TestGoogleOAuth_InterfaceSatisfaction(t *testing.T) {
	var _ domain.OAuthProvider = (*oauth.GoogleOAuth)(nil) //nolint:unused
}

// =============================================================================
// GetAuthURL — verifica que genera URL de Google con parámetros correctos
// =============================================================================

func TestGetAuthURL_ParametrosCorrectos(t *testing.T) {
	cfg := config.OAuthConfig{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		GoogleRedirectURL:  "http://localhost:8080/v1/auth/oauth/google/callback",
	}
	provider := oauth.NewGoogleOAuth(cfg)

	state := "test-state-token"
	codeChallenge := "test-code-challenge"
	authURL := provider.GetAuthURL(state, codeChallenge)

	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("GetAuthURL retornó URL inválida: %v", err)
	}

	q := u.Query()

	tests := []struct {
		param    string
		esperado string
	}{
		{"client_id", "test-client-id"},
		{"redirect_uri", "http://localhost:8080/v1/auth/oauth/google/callback"},
		{"response_type", "code"},
		{"scope", "openid email profile"},
		{"state", state},
		{"code_challenge", codeChallenge},
		{"code_challenge_method", "S256"},
		{"access_type", "offline"},
		{"prompt", "consent"},
	}

	for _, tt := range tests {
		t.Run("param_"+tt.param, func(t *testing.T) {
			got := q.Get(tt.param)
			if got != tt.esperado {
				t.Errorf("parámetro %s: esperaba %q, obtuve %q", tt.param, tt.esperado, got)
			}
		})
	}

	// Verificar que la URL base es correcta
	baseURL := u.Scheme + "://" + u.Host + u.Path
	if baseURL != oauth.GoogleAuthURL {
		t.Errorf("URL base: esperaba %q, obtuve %q", oauth.GoogleAuthURL, baseURL)
	}
}

// =============================================================================
// ExchangeCode — éxito: mock del token endpoint de Google
// =============================================================================

func TestExchangeCode_Exito(t *testing.T) {
	ctx := t.Context()

	mockToken := map[string]any{
		"access_token":  "ya29.test-access-token",
		"refresh_token": "1//test-refresh-token",
		"expires_in":    3599,
		"token_type":    "Bearer",
	}

	rt := newGoogleRoundTripper(t, oauth.GoogleTokenURL, http.StatusOK, mockToken)
	provider := oauth.NewGoogleOAuthWithClient("test-client-id", "test-secret", &http.Client{Transport: rt})

	token, err := provider.ExchangeCode(ctx, "test-auth-code", "test-code-verifier")
	if err != nil {
		t.Fatalf("ExchangeCode retornó error inesperado: %v", err)
	}

	if token.AccessToken != "ya29.test-access-token" {
		t.Errorf("AccessToken: esperaba %q, obtuve %q", "ya29.test-access-token", token.AccessToken)
	}
	if token.RefreshToken != "1//test-refresh-token" {
		t.Errorf("RefreshToken: esperaba %q, obtuve %q", "1//test-refresh-token", token.RefreshToken)
	}
	if token.ExpiresIn != 3599 {
		t.Errorf("ExpiresIn: esperaba %d, obtuve %d", 3599, token.ExpiresIn)
	}
	if token.TokenType != "Bearer" {
		t.Errorf("TokenType: esperaba %q, obtuve %q", "Bearer", token.TokenType)
	}

	// Verificar que se envió el body correcto
	body := rt.lastBody
	if !strings.Contains(body, "code=test-auth-code") {
		t.Errorf("body no contiene code=test-auth-code: %s", body)
	}
	if !strings.Contains(body, "grant_type=authorization_code") {
		t.Errorf("body no contiene grant_type=authorization_code: %s", body)
	}
	if !strings.Contains(body, "code_verifier=test-code-verifier") {
		t.Errorf("body no contiene code_verifier=test-code-verifier: %s", body)
	}
}

// =============================================================================
// ExchangeCode — error: mock retorna HTTP no-OK
// =============================================================================

func TestExchangeCode_ErrorHTTP(t *testing.T) {
	ctx := t.Context()

	rt := newGoogleRoundTripper(t, oauth.GoogleTokenURL, http.StatusBadRequest,
		map[string]string{"error": "invalid_grant"})
	provider := oauth.NewGoogleOAuthWithClient("test-client-id", "test-secret", &http.Client{Transport: rt})

	_, err := provider.ExchangeCode(ctx, "bad-code", "test-code-verifier")
	if err == nil {
		t.Fatal("esperaba error para respuesta HTTP no-OK")
	}
	if !errors.Is(err, domain.ErrOAuthExchangeFailed) {
		t.Errorf("esperaba ErrOAuthExchangeFailed, obtuve %v", err)
	}
}

// =============================================================================
// ExchangeCode — error: conexión rechazada (sin servidor)
// =============================================================================

func TestExchangeCode_ErrorConexion(t *testing.T) {
	ctx := t.Context()

	// http.Client con Transport que rechaza conexiones
	provider := oauth.NewGoogleOAuthWithClient("test-client-id", "test-secret", &http.Client{
		Transport: &failingTransport{},
	})

	_, err := provider.ExchangeCode(ctx, "test-code", "test-verifier")
	if err == nil {
		t.Fatal("esperaba error de conexión")
	}
	if !errors.Is(err, domain.ErrOAuthExchangeFailed) {
		t.Errorf("esperaba ErrOAuthExchangeFailed, obtuve %v", err)
	}
}

// =============================================================================
// GetUserInfo — éxito: mock del userinfo endpoint de Google
// =============================================================================

func TestGetUserInfo_Exito(t *testing.T) {
	ctx := t.Context()

	mockUser := map[string]any{
		"sub":            "1234567890",
		"email":          "usuario@gmail.com",
		"email_verified": true,
		"name":           "Usuario Test",
		"picture":        "https://lh3.googleusercontent.com/photo.jpg",
	}

	rt := newGoogleRoundTripper(t, oauth.GoogleUserInfoURL, http.StatusOK, mockUser)
	provider := oauth.NewGoogleOAuthWithClient("test-client-id", "test-secret", &http.Client{Transport: rt})

	userInfo, err := provider.GetUserInfo(ctx, "ya29.test-access-token")
	if err != nil {
		t.Fatalf("GetUserInfo retornó error inesperado: %v", err)
	}

	if userInfo.ProviderUserID != "1234567890" {
		t.Errorf("ProviderUserID: esperaba %q, obtuve %q", "1234567890", userInfo.ProviderUserID)
	}
	if userInfo.Email != "usuario@gmail.com" {
		t.Errorf("Email: esperaba %q, obtuve %q", "usuario@gmail.com", userInfo.Email)
	}
	if !userInfo.EmailVerified {
		t.Error("EmailVerified: esperaba true")
	}
	if userInfo.Name != "Usuario Test" {
		t.Errorf("Name: esperaba %q, obtuve %q", "Usuario Test", userInfo.Name)
	}
	if userInfo.Picture != "https://lh3.googleusercontent.com/photo.jpg" {
		t.Errorf("Picture: esperaba %q, obtuve %q", "https://lh3.googleusercontent.com/photo.jpg", userInfo.Picture)
	}

	// Verificar Authorization header
	if rt.lastAuthHeader != "Bearer ya29.test-access-token" {
		t.Errorf("Authorization header: esperaba %q, obtuve %q",
			"Bearer ya29.test-access-token", rt.lastAuthHeader)
	}
}

// =============================================================================
// GetUserInfo — error: servidor retorna HTTP no-OK
// =============================================================================

func TestGetUserInfo_ErrorHTTP(t *testing.T) {
	ctx := t.Context()

	rt := newGoogleRoundTripper(t, oauth.GoogleUserInfoURL, http.StatusUnauthorized,
		map[string]string{"error": "invalid_token"})
	provider := oauth.NewGoogleOAuthWithClient("test-client-id", "test-secret", &http.Client{Transport: rt})

	_, err := provider.GetUserInfo(ctx, "token-invalido")
	if err == nil {
		t.Fatal("esperaba error para respuesta HTTP no-OK")
	}
	if !errors.Is(err, domain.ErrOAuthExchangeFailed) {
		t.Errorf("esperaba ErrOAuthExchangeFailed, obtuve %v", err)
	}
}

// =============================================================================
// Helpers
// =============================================================================

// googleRoundTripper intercepta requests a una URL específica y devuelve
// una respuesta JSON mockeada. Requests a otras URLs pasan por DefaultTransport.
type googleRoundTripper struct {
	t              *testing.T
	interceptURL   string
	statusCode     int
	responseBody   any
	lastBody       string
	lastAuthHeader string
}

func newGoogleRoundTripper(t *testing.T, interceptURL string, statusCode int, responseBody any) *googleRoundTripper {
	t.Helper()
	return &googleRoundTripper{
		t:            t,
		interceptURL: interceptURL,
		statusCode:   statusCode,
		responseBody: responseBody,
	}
}

func (rt *googleRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() == rt.interceptURL {
		// Capturar body y headers para aserciones
		if req.Body != nil {
			bodyBytes, _ := io.ReadAll(req.Body)
			req.Body.Close()
			rt.lastBody = string(bodyBytes)
			// Re-crear body para que el handler pueda leerlo si es necesario
			req.Body = io.NopCloser(strings.NewReader(rt.lastBody))
		}
		rt.lastAuthHeader = req.Header.Get("Authorization")

		// Serializar respuesta mock
		bodyJSON, err := json.Marshal(rt.responseBody)
		if err != nil {
			rt.t.Fatalf("error serializando mock response: %v", err)
		}

		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Type", "application/json")
		rec.WriteHeader(rt.statusCode)
		rec.Write(bodyJSON)

		return rec.Result(), nil
	}

	// Requests no interceptadas usan DefaultTransport
	return http.DefaultTransport.RoundTrip(req)
}

// failingTransport siempre retorna error de conexión.
type failingTransport struct{}

func (f *failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("conexión rechazada")
}
