package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/ProacTrip/Backend/internal/config"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
)

// GoogleOAuth implementa domain.OAuthProvider para Google OAuth 2.0.
// Usa únicamente net/http + encoding/json de la stdlib, sin dependencias externas.
//
// Compile-time check para asegurar que satisface la interfaz.
var _ domain.OAuthProvider = (*GoogleOAuth)(nil)

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
	googleScope       = "openid email profile"
)

type GoogleOAuth struct {
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client
}

// NewGoogleOAuth crea un nuevo proveedor Google OAuth a partir de la configuración.
func NewGoogleOAuth(cfg config.OAuthConfig) *GoogleOAuth {
	return &GoogleOAuth{
		clientID:     cfg.GoogleClientID,
		clientSecret: cfg.GoogleClientSecret,
		redirectURL:  cfg.GoogleRedirectURL,
		httpClient:   &http.Client{},
	}
}

// GetAuthURL construye la URL de autorización de Google OAuth 2.0.
// Incluye PKCE (S256) y state como parámetros.
func (g *GoogleOAuth) GetAuthURL(state, codeChallenge string) string {
	u, _ := url.Parse(googleAuthURL)
	q := u.Query()
	q.Set("client_id", g.clientID)
	q.Set("redirect_uri", g.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", googleScope)
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("access_type", "offline")          // refresh token
	q.Set("prompt", "consent")               // forzar consentimiento para obtener refresh_token
	u.RawQuery = q.Encode()
	return u.String()
}

// ExchangeCode intercambia un código de autorización por un token OAuth.
// Realiza POST a https://oauth2.googleapis.com/token.
func (g *GoogleOAuth) ExchangeCode(ctx context.Context, code, codeVerifier string) (*domain.OAuthToken, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"redirect_uri":  {g.redirectURL},
		"grant_type":    {"authorization_code"},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("crear request token exchange: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "error al intercambiar código OAuth",
			slog.String("error", err.Error()),
		)
		return nil, domain.ErrOAuthExchangeFailed
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		slog.ErrorContext(ctx, "respuesta no exitosa del token endpoint",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(bodyBytes)),
		)
		return nil, domain.ErrOAuthExchangeFailed
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decodificar respuesta token: %w", err)
	}

	return &domain.OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		TokenType:    tokenResp.TokenType,
	}, nil
}

// GetUserInfo obtiene la información del usuario autenticado de Google.
// Realiza GET a https://www.googleapis.com/oauth2/v3/userinfo.
func (g *GoogleOAuth) GetUserInfo(ctx context.Context, accessToken string) (*domain.OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("crear request userinfo: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "error al obtener userinfo de Google",
			slog.String("error", err.Error()),
		)
		return nil, domain.ErrOAuthExchangeFailed
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		slog.ErrorContext(ctx, "respuesta no exitosa del userinfo endpoint",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(bodyBytes)),
		)
		return nil, domain.ErrOAuthExchangeFailed
	}

	var userInfo struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decodificar respuesta userinfo: %w", err)
	}

	return &domain.OAuthUserInfo{
		ProviderUserID: userInfo.Sub,
		Email:          userInfo.Email,
		EmailVerified:  userInfo.EmailVerified,
		Name:           userInfo.Name,
		Picture:        userInfo.Picture,
	}, nil
}
