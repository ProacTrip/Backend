package oauth

import "net/http"

// Exports for black-box testing (S4 convention).

// NewGoogleOAuthWithClient creates a GoogleOAuth with a custom HTTP client for testing.
func NewGoogleOAuthWithClient(clientID, clientSecret string, httpClient *http.Client) *GoogleOAuth {
	return &GoogleOAuth{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  "http://localhost:8080/v1/auth/oauth/google/callback",
		httpClient:   httpClient,
	}
}

// URL constants for testing.
const (
	GoogleAuthURL     = googleAuthURL
	GoogleTokenURL    = googleTokenURL
	GoogleUserInfoURL = googleUserInfoURL
)
