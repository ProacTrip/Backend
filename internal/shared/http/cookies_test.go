package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	sharedhttp "github.com/ProacTrip/Backend/internal/shared/http"
)

// newCookieTestContext crea un contexto Echo con httptest.ResponseRecorder.
func newCookieTestContext() (*echo.Echo, *echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return e, c, rec
}

// =============================================================================
// T4.1.1: SetAuthCookiesFromTokens prod — atributos de cookie correctos
// =============================================================================

func TestSetAuthCookiesFromTokens_Prod_AtributosCorrectos(t *testing.T) {
	tests := []struct {
		name         string
		accessToken  string
		refreshToken string
		cookieDomain string
	}{
		{
			name:         "con_dominio_proactrip",
			accessToken:  "v5.local.access123",
			refreshToken: "v5.local.refresh456",
			cookieDomain: ".proactrip.com",
		},
		{
			name:         "sin_dominio",
			accessToken:  "v5.local.access789",
			refreshToken: "v5.local.refresh012",
			cookieDomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, c, rec := newCookieTestContext()
			defer func() { _ = e }()

			err := sharedhttp.SetAuthCookiesFromTokens(c, tt.accessToken, tt.refreshToken, tt.cookieDomain)
			if err != nil {
				t.Fatalf("SetAuthCookiesFromTokens devolvió error: %v", err)
			}

			cookies := rec.Result().Cookies()
			if len(cookies) < 2 {
				t.Fatalf("esperaba al menos 2 cookies, obtuve %d", len(cookies))
			}

			// Buscar cookies por nombre
			var accessCookie, refreshCookie *http.Cookie
			for _, ck := range cookies {
				switch ck.Name {
				case "__Secure-access_token":
					accessCookie = ck
				case "__Secure-refresh_token":
					refreshCookie = ck
				}
			}

			if accessCookie == nil {
				t.Fatal("cookie __Secure-access_token no encontrada")
			}
			if refreshCookie == nil {
				t.Fatal("cookie __Secure-refresh_token no encontrada")
			}

			// --- Access cookie ---
			if !accessCookie.HttpOnly {
				t.Error("access cookie debe tener HttpOnly=true")
			}
			if !accessCookie.Secure {
				t.Error("access cookie debe tener Secure=true")
			}
			if accessCookie.SameSite != http.SameSiteLaxMode {
				t.Errorf("access cookie SameSite = %v, want %v", accessCookie.SameSite, http.SameSiteLaxMode)
			}
			if accessCookie.Path != "/" {
				t.Errorf("access cookie Path = %q, want %q", accessCookie.Path, "/")
			}
			if accessCookie.MaxAge != 900 {
				t.Errorf("access cookie MaxAge = %d, want 900 (15 min)", accessCookie.MaxAge)
			}
			if accessCookie.Value != tt.accessToken {
				t.Errorf("access cookie Value = %q, want %q", accessCookie.Value, tt.accessToken)
			}

			// Domain: http.Cookie.String() normaliza el dominio (quita el leading dot).
			// ".proactrip.com" → "proactrip.com" en el header Set-Cookie.
			expectDomain := tt.cookieDomain
			if len(expectDomain) > 0 && expectDomain[0] == '.' {
				expectDomain = expectDomain[1:]
			}
			if tt.cookieDomain != "" {
				if accessCookie.Domain != expectDomain {
					t.Errorf("access cookie Domain = %q, want %q", accessCookie.Domain, expectDomain)
				}
			} else {
				if accessCookie.Domain != "" {
					t.Errorf("access cookie NO debe tener Domain cuando cookieDomain es vacío, pero tiene %q", accessCookie.Domain)
				}
			}

			// --- Refresh cookie ---
			if !refreshCookie.HttpOnly {
				t.Error("refresh cookie debe tener HttpOnly=true")
			}
			if !refreshCookie.Secure {
				t.Error("refresh cookie debe tener Secure=true")
			}
			if refreshCookie.SameSite != http.SameSiteLaxMode {
				t.Errorf("refresh cookie SameSite = %v, want %v", refreshCookie.SameSite, http.SameSiteLaxMode)
			}
			if refreshCookie.Path != "/" {
				t.Errorf("refresh cookie Path = %q, want %q", refreshCookie.Path, "/")
			}
			if refreshCookie.MaxAge != 604800 {
				t.Errorf("refresh cookie MaxAge = %d, want 604800 (7 días)", refreshCookie.MaxAge)
			}
			if refreshCookie.Value != tt.refreshToken {
				t.Errorf("refresh cookie Value = %q, want %q", refreshCookie.Value, tt.refreshToken)
			}
		})
	}
}

// =============================================================================
// T4.1.2: SetAuthCookiesDev — sin Secure, sin prefijo __Secure-, sin Domain
// =============================================================================

func TestSetAuthCookiesDev_SinSecure_SinPrefijo(t *testing.T) {
	e, c, rec := newCookieTestContext()
	defer func() { _ = e }()

	err := sharedhttp.SetAuthCookiesDev(c, "access_dev_123", "refresh_dev_456")
	if err != nil {
		t.Fatalf("SetAuthCookiesDev devolvió error: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) < 2 {
		t.Fatalf("esperaba al menos 2 cookies, obtuve %d", len(cookies))
	}

	var accessCookie, refreshCookie *http.Cookie
	for _, ck := range cookies {
		switch ck.Name {
		case "access_token":
			accessCookie = ck
		case "refresh_token":
			refreshCookie = ck
		}
	}

	if accessCookie == nil {
		t.Fatal("cookie access_token no encontrada en dev")
	}
	if refreshCookie == nil {
		t.Fatal("cookie refresh_token no encontrada en dev")
	}

	// Verificar que NO tiene prefijo __Secure-
	// (ya verificado por el nombre "access_token" sin prefijo)

	// Verificar sin Secure
	if accessCookie.Secure {
		t.Error("dev access cookie NO debe tener Secure=true")
	}
	if refreshCookie.Secure {
		t.Error("dev refresh cookie NO debe tener Secure=true")
	}

	// Verificar sin Domain
	if accessCookie.Domain != "" {
		t.Errorf("dev access cookie no debe tener Domain, pero tiene %q", accessCookie.Domain)
	}
	if refreshCookie.Domain != "" {
		t.Errorf("dev refresh cookie no debe tener Domain, pero tiene %q", refreshCookie.Domain)
	}

	// HttpOnly debe seguir presente
	if !accessCookie.HttpOnly {
		t.Error("dev access cookie debe tener HttpOnly=true")
	}
	if !refreshCookie.HttpOnly {
		t.Error("dev refresh cookie debe tener HttpOnly=true")
	}

	// SameSite debe ser Lax
	if accessCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("dev access cookie SameSite = %v, want %v", accessCookie.SameSite, http.SameSiteLaxMode)
	}

	// Path debe ser /
	if accessCookie.Path != "/" {
		t.Errorf("dev access cookie Path = %q, want %q", accessCookie.Path, "/")
	}

	// MaxAge correcto
	if accessCookie.MaxAge != 900 {
		t.Errorf("dev access cookie MaxAge = %d, want 900", accessCookie.MaxAge)
	}
	if refreshCookie.MaxAge != 604800 {
		t.Errorf("dev refresh cookie MaxAge = %d, want 604800", refreshCookie.MaxAge)
	}
}

// =============================================================================
// T4.1.3: ClearAuthCookies — MaxAge=0, Clear-Site-Data header presente
// =============================================================================

func TestClearAuthCookies_MaxAgeCero_ClearSiteDataHeader(t *testing.T) {
	e, c, rec := newCookieTestContext()
	defer func() { _ = e }()

	err := sharedhttp.ClearAuthCookies(c, ".proactrip.com")
	if err != nil {
		t.Fatalf("ClearAuthCookies devolvió error: %v", err)
	}

	// Verificar Clear-Site-Data header
	clearSiteData := rec.Header().Get("Clear-Site-Data")
	if clearSiteData != `"cookies"` {
		t.Errorf("Clear-Site-Data = %q, want %q", clearSiteData, `"cookies"`)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) < 2 {
		t.Fatalf("esperaba al menos 2 cookies limpiadas, obtuve %d", len(cookies))
	}

	var accessCookie, refreshCookie *http.Cookie
	for _, ck := range cookies {
		switch ck.Name {
		case "__Secure-access_token":
			accessCookie = ck
		case "__Secure-refresh_token":
			refreshCookie = ck
		}
	}

	if accessCookie == nil {
		t.Fatal("cookie __Secure-access_token no encontrada en clear")
	}
	if refreshCookie == nil {
		t.Fatal("cookie __Secure-refresh_token no encontrada en clear")
	}

	// MaxAge = -1 → generates Max-Age=0 (delete cookie immediately)
	if accessCookie.MaxAge != -1 {
		t.Errorf("clear access cookie MaxAge = %d, want -1", accessCookie.MaxAge)
	}
	if refreshCookie.MaxAge != -1 {
		t.Errorf("clear refresh cookie MaxAge = %d, want -1", refreshCookie.MaxAge)
	}

	// Value vacío
	if accessCookie.Value != "" {
		t.Errorf("clear access cookie Value = %q, want empty", accessCookie.Value)
	}
	if refreshCookie.Value != "" {
		t.Errorf("clear refresh cookie Value = %q, want empty", refreshCookie.Value)
	}

	// Domain preservado (http.Cookie.String() normaliza quitando el leading dot)
	if accessCookie.Domain != "proactrip.com" {
		t.Errorf("clear access cookie Domain = %q, want %q", accessCookie.Domain, "proactrip.com")
	}
}

// =============================================================================
// T4.1.3b: ClearAuthCookies sin dominio — Domain no presente
// =============================================================================

func TestClearAuthCookies_SinDominio_DomainNoPresente(t *testing.T) {
	e, c, rec := newCookieTestContext()
	defer func() { _ = e }()

	err := sharedhttp.ClearAuthCookies(c, "")
	if err != nil {
		t.Fatalf("ClearAuthCookies devolvió error: %v", err)
	}

	cookies := rec.Result().Cookies()
	var accessCookie *http.Cookie
	for _, ck := range cookies {
		if ck.Name == "__Secure-access_token" {
			accessCookie = ck
			break
		}
	}

	if accessCookie == nil {
		t.Fatal("cookie __Secure-access_token no encontrada en clear sin dominio")
	}
	if accessCookie.Domain != "" {
		t.Errorf("clear cookie sin dominio no debe tener Domain, pero tiene %q", accessCookie.Domain)
	}
	if accessCookie.MaxAge != -1 {
		t.Errorf("clear cookie MaxAge = %d, want -1", accessCookie.MaxAge)
	}
}

// =============================================================================
// T4.1.4: ClearAuthCookiesDev — limpia los nombres correctos para dev
// =============================================================================

func TestClearAuthCookiesDev_NombresCorrectos_MaxAgeCero(t *testing.T) {
	e, c, rec := newCookieTestContext()
	defer func() { _ = e }()

	err := sharedhttp.ClearAuthCookiesDev(c)
	if err != nil {
		t.Fatalf("ClearAuthCookiesDev devolvió error: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) < 2 {
		t.Fatalf("esperaba al menos 2 cookies dev limpiadas, obtuve %d", len(cookies))
	}

	cookieNames := make(map[string]*http.Cookie)
	for _, ck := range cookies {
		cookieNames[ck.Name] = ck
	}

	// Verificar nombres dev (sin prefijo __Secure-)
	accessCookie, ok := cookieNames["access_token"]
	if !ok {
		t.Error("ClearAuthCookiesDev debe limpiar cookie 'access_token'")
	} else {
		if accessCookie.MaxAge != -1 {
			t.Errorf("dev clear access_token MaxAge = %d, want -1 (generates Max-Age=0)", accessCookie.MaxAge)
		}
		if accessCookie.Value != "" {
			t.Errorf("dev clear access_token Value = %q, want empty", accessCookie.Value)
		}
		if !accessCookie.HttpOnly {
			t.Error("dev clear access_token debe tener HttpOnly=true")
		}
	}

	refreshCookie, ok := cookieNames["refresh_token"]
	if !ok {
		t.Error("ClearAuthCookiesDev debe limpiar cookie 'refresh_token'")
	} else {
		if refreshCookie.MaxAge != -1 {
			t.Errorf("dev clear refresh_token MaxAge = %d, want -1 (generates Max-Age=0)", refreshCookie.MaxAge)
		}
		if refreshCookie.Value != "" {
			t.Errorf("dev clear refresh_token Value = %q, want empty", refreshCookie.Value)
		}
		if !refreshCookie.HttpOnly {
			t.Error("dev clear refresh_token debe tener HttpOnly=true")
		}
	}

	// ClearAuthCookiesDev NO debe enviar Clear-Site-Data header
	clearSiteData := rec.Header().Get("Clear-Site-Data")
	if clearSiteData != "" {
		t.Errorf("ClearAuthCookiesDev NO debe enviar Clear-Site-Data header, pero envió %q", clearSiteData)
	}

	// Verificar que NO hay cookies con prefijo __Secure- en dev
	if _, hasSecure := cookieNames["__Secure-access_token"]; hasSecure {
		t.Error("ClearAuthCookiesDev no debe limpiar cookies con prefijo __Secure-")
	}
}
