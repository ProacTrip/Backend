package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/alicebob/miniredis/v2"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"
)

func setupEchoWithRateLimiter(t *testing.T, limit int) (*echo.Echo, *ratelimit.RateLimiter, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { rdb.Close() })

	cfg := ratelimit.DefaultConfig()
	cfg.GlobalPerMinute = limit
	cfg.AnonymousPerMinute = limit
	cfg.AuthenticatedPerMinute = limit

	rl := ratelimit.NewRateLimiter(rdb, cfg)

	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		he, ok := err.(*echo.HTTPError)
		if !ok {
			c.String(http.StatusInternalServerError, "internal error")
			return
		}
		c.JSON(he.Code, map[string]string{"error": he.Message})
	}

	return e, rl, mr
}

func TestGlobalRateLimitMiddlewareAllowsUnderLimit(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 10)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, ratelimit.GlobalRateLimitMiddleware(rl))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %s, want ok", rec.Body.String())
	}
}

func TestGlobalRateLimitMiddlewareSetsHeaders(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 10)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, ratelimit.GlobalRateLimitMiddleware(rl))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Header().Get("RateLimit-Limit") != "10" {
		t.Errorf("RateLimit-Limit = %s, want 10", rec.Header().Get("RateLimit-Limit"))
	}
	if rec.Header().Get("RateLimit-Remaining") != "9" {
		t.Errorf("RateLimit-Remaining = %s, want 9", rec.Header().Get("RateLimit-Remaining"))
	}
	if rec.Header().Get("RateLimit-Reset") == "" {
		t.Error("RateLimit-Reset header missing")
	}
}

func TestGlobalRateLimitMiddlewareReturns429WhenExceeded(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 1)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, ratelimit.GlobalRateLimitMiddleware(rl))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", rec2.Code)
	}
}

func TestAnonymousRateLimitMiddleware(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 2)

	anonMW := ratelimit.AnonymousCookieMiddleware(nil, false)
	anonRLMW := ratelimit.AnonymousRateLimitMiddleware(rl)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, anonMW, anonRLMW)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "__Secure-anon_token" {
		t.Errorf("expected anon_id cookie, got %d cookies", len(cookies))
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAnonymousCookieMiddlewareSetsCookieAttributes(t *testing.T) {
	e, _, _ := setupEchoWithRateLimiter(t, 10)

	anonMW := ratelimit.AnonymousCookieMiddleware(nil, false)
	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, anonMW)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookies set")
	}

	cookie := cookies[0]
	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if !cookie.Secure {
		t.Error("cookie should be Secure")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %d, want Lax", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("cookie Path = %s, want /", cookie.Path)
	}
	if cookie.Value == "" {
		t.Error("cookie value should not be empty")
	}
}

func TestAnonymousCookieMiddlewareReusesExistingCookie(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 10)

	anonMW := ratelimit.AnonymousCookieMiddleware(nil, false)
	anonRLMW := ratelimit.AnonymousRateLimitMiddleware(rl)

	e.GET("/test", func(c *echo.Context) error {
		anonID := ratelimit.AnonIDFromContext(c)
		return c.String(http.StatusOK, anonID)
	}, anonMW, anonRLMW)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  "__Secure-anon_token",
		Value: "existing-anon-id-123",
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Body.String() != "existing-anon-id-123" {
		t.Errorf("body = %s, want existing-anon-id-123", rec.Body.String())
	}
}

func TestProviderRateLimitMiddleware(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 10)

	serpapiMW := ratelimit.ProviderRateLimitMiddleware(rl, "serpapi")
	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, serpapiMW)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuthenticatedRateLimitMiddlewareSkipsWithoutToken(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 10)

	authMW := ratelimit.AuthenticatedRateLimitMiddleware(rl,
		func(c *echo.Context) (string, bool) {
			return "", false
		},
	)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, authMW)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (should skip without token)", rec.Code)
	}
}

func TestAuthenticatedRateLimitMiddlewareAppliesWithToken(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 1)

	authMW := ratelimit.AuthenticatedRateLimitMiddleware(rl,
		func(c *echo.Context) (string, bool) {
			return "user-uuid-test", true
		},
	)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, authMW)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: status = %d, want 429", rec2.Code)
	}
}

func TestGlobalRateLimitMiddlewareIndependentPerIP(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 1)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, ratelimit.GlobalRateLimitMiddleware(rl))

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "1.1.1.1:12345"
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("ip 1.1.1.1 first request: status = %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "2.2.2.2:12345"
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("ip 2.2.2.2 should be allowed (independent limit), but got %d", rec2.Code)
	}
}

func TestProviderRateLimitMiddlewareDifferentProvidersIndependent(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 10)

	serpapiMW := ratelimit.ProviderRateLimitMiddleware(rl, "serpapi")
	resendMW := ratelimit.ProviderRateLimitMiddleware(rl, "resend")

	e.GET("/serpapi", func(c *echo.Context) error {
		return c.String(http.StatusOK, "serp")
	}, serpapiMW)
	e.GET("/resend", func(c *echo.Context) error {
		return c.String(http.StatusOK, "resend")
	}, resendMW)

	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/serpapi", nil))
	if rec1.Code != http.StatusOK {
		t.Fatalf("serpapi first: status = %d", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/resend", nil))
	if rec2.Code != http.StatusOK {
		t.Errorf("resend should be allowed (independent from serpapi), but got %d", rec2.Code)
	}
}

func TestAnonymousRateLimitMiddlewareReturns429Exceeded(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 1)

	anonMW := ratelimit.AnonymousCookieMiddleware(nil, false)
	anonRLMW := ratelimit.AnonymousRateLimitMiddleware(rl)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, anonMW, anonRLMW)

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.AddCookie(&http.Cookie{Name: "__Secure-anon_token", Value: "overlimit-anon"})
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.AddCookie(&http.Cookie{Name: "__Secure-anon_token", Value: "overlimit-anon"})
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: status = %d, want 429", rec2.Code)
	}
}

func TestMiddlewareRemainingDecrements(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 5)

	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, ratelimit.GlobalRateLimitMiddleware(rl))

	expectedRemaining := []string{"4", "3", "2", "1", "0"}
	for i, want := range expectedRemaining {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.100:12345"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		got := rec.Header().Get("RateLimit-Remaining")
		if got != want {
			t.Errorf("request %d: RateLimit-Remaining = %s, want %s", i, got, want)
			break
		}
	}
}

func TestEchoContextMultipleMiddlewareComposition(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 10)

	e.GET("/composed",
		func(c *echo.Context) error {
			return c.String(http.StatusOK, "composed")
		},
		ratelimit.GlobalRateLimitMiddleware(rl),
		ratelimit.AnonymousCookieMiddleware(nil, false),
		ratelimit.AnonymousRateLimitMiddleware(rl),
	)

	req := httptest.NewRequest(http.MethodGet, "/composed", nil)
	req.RemoteAddr = "192.168.1.10:12345"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "composed" {
		t.Errorf("body = %s, want composed", rec.Body.String())
	}
	if rec.Header().Get("RateLimit-Limit") == "" {
		t.Error("RateLimit-Limit header missing")
	}
}

func TestResponseHeadersPresent(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 10)

	e.GET("/headers", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, ratelimit.GlobalRateLimitMiddleware(rl))

	req := httptest.NewRequest(http.MethodGet, "/headers", nil)
	req.RemoteAddr = "172.16.0.1:12345"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	requiredHeaders := []string{"RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset"}
	for _, h := range requiredHeaders {
		if rec.Header().Get(h) == "" {
			t.Errorf("header %s missing", h)
		}
	}
}

func TestAnonymousRateLimitNoAnonIDSkips(t *testing.T) {
	e, rl, _ := setupEchoWithRateLimiter(t, 1)

	anonRLMW := ratelimit.AnonymousRateLimitMiddleware(rl)

	e.GET("/no-anon", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, anonRLMW)

	req := httptest.NewRequest(http.MethodGet, "/no-anon", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (skip without anon cookie)", rec.Code)
	}
}

func TestConfigLoadsValidValues(t *testing.T) {
	cfg := ratelimit.LoadRateLimitConfig()
	if cfg.GlobalPerMinute <= 0 {
		t.Errorf("GlobalPerMinute = %d, should be > 0", cfg.GlobalPerMinute)
	}
	if cfg.AuthenticatedPerMinute <= 0 {
		t.Errorf("AuthenticatedPerMinute = %d, should be > 0", cfg.AuthenticatedPerMinute)
	}
	if cfg.AnonymousPerMinute <= 0 {
		t.Errorf("AnonymousPerMinute = %d, should be > 0", cfg.AnonymousPerMinute)
	}
}

func BenchmarkRateLimiterAllow(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	cfg := ratelimit.DefaultConfig()
	rl := ratelimit.NewRateLimiter(rdb, cfg)

	ctx := b.Context()
	b.ResetTimer()
	for b.Loop() {
		rl.Allow(ctx, "bench:ip", 1000000, 60)
	}
}
