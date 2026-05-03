# middleware-echo

## Trigger
When the user needs to create Echo v5 middleware for auth, rate limiting, security headers, or custom HTTP processing. Keywords: "middleware", "echo middleware", "auth middleware", "rate limit middleware", "security headers", "CSP", "HSTS", "token validation"

## Questions to Ask (ALWAYS ask first — never generate code without answers)

1. ¿Qué tipo de middleware necesitás? (auth con cookies, rate limiting, security headers, custom)
2. ¿Para qué grupo de rutas se aplica? (públicas, autenticadas, proveedores externos, todas)
3. ¿Qué configuración requiere? (cookie domain, rate limit tiers, API keys, secrets)
4. ¿El middleware necesita acceder a servicios del módulo? (token service, user repository)
5. ¿Debe aplicarse globalmente (`e.Use()`) o a rutas específicas (`group.Use()`)?

## Rules (Non-Negotiable — fail if violated)

### Middleware-Specific Rules (G1-G4 from spec)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| G1 | Auth | MUST | Extract token from cookie (__Secure-access_token or access_token), never from Authorization header |
| G2 | Rate Limiting | MUST | Four-tier: Global(IP) → Anonymous(cookie) → Authenticated(userID) → Provider(provider name) |
| G3 | Security Headers | MUST | Set CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy on every response |
| G4 | MiddlewareFunc | MUST | Return `echo.MiddlewareFunc` — signature: `func(next echo.HandlerFunc) echo.HandlerFunc` |

### Global Rules (R1-R9)
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| R1 | Module Isolation | CRITICAL | Modules communicate via injected interfaces or published events |
| R3 | Shared Boundaries | CRITICAL | shared/ packages MUST NOT import from modules/ |
| R5 | DI | MUST | Manual constructor injection, zero globals |
| R7 | Go 1.26 | MUST | omitzero, new(expr), errors.AsType |
| R8 | Echo v5 | MUST | *echo.Context pointer, echo.StartConfig |

### Additional Conventions
| Rule | Category | Severity | Check |
|------|----------|----------|-------|
| M1 | Cookie Auth | MUST | Read from cookie, never Authorization header. Use __Secure- prefix in production |
| M2 | Tiered Rate Limit | MUST | Global → Anonymous → Authenticated → Provider fallback chain |
| M3 | Security Headers Set | MUST | CSP: default-src 'self', HSTS: max-age=31536000, X-Frame-Options: DENY, X-Content-Type-Options: nosniff, Referrer-Policy: strict-origin-when-cross-origin |
| M4 | MiddlewareFunc | MUST | All middleware functions return echo.MiddlewareFunc compatible signature |
| M5 | Refresh Rotation | SHOULD | Auth middleware silently rotates tokens on access expiry |
| M6 | Rate Limit Headers | MUST | Set RateLimit-Limit, RateLimit-Remaining, RateLimit-Reset on every response; Retry-After on 429 |
| M7 | Config struct | MUST | Constructor receives Config struct, not individual params |
| M8 | Error Handling | MUST | Never silently swallow errors — log with slog.ErrorContext or return proper error |

## Patterns

### Pattern 1: Auth Middleware Structure (from auth/adapters/middleware/echo.go)
```go
type AuthMiddleware struct {
    config AuthConfig
}
type AuthConfig struct {
    IsProduction bool
    TokenSvc     TokenService
    CookieDomain string
}
func NewAuthMiddleware(cfg AuthConfig) *AuthMiddleware {
    return &AuthMiddleware{config: cfg}
}
```

### Pattern 2: Security Headers Middleware (from shared/middleware/security_headers.go)
```go
// Security headers middleware pattern — NO Partitioned
// Partitioned (CHIPS) no se usa con Domain amplio en subdominios. SameSite=Lax + Domain=.proactrip.com es suficiente.
func SecurityHeaders() echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c *echo.Context) error {
            c.Response().Header().Set("Content-Security-Policy", "default-src 'self'")
            c.Response().Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
            c.Response().Header().Set("X-Frame-Options", "DENY")
            c.Response().Header().Set("X-Content-Type-Options", "nosniff")
            c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
            return next(c)
        }
    }
}
```

### Pattern 3: Rate Limit Middleware Tier Chain (from ratelimit/middleware.go)
```go
func GlobalRateLimitMiddleware(rl *RateLimiter) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c *echo.Context) error {
            result, err := rl.GlobalAllow(c.Request().Context(), c.RealIP())
            if err != nil || !result.Allowed {
                return echo.NewHTTPError(429, "rate limit exceeded")
            }
            return next(c)
        }
    }
}
```

## Templates

### Template: Auth Middleware (struct-based)
```go
package middleware

type AuthMiddleware struct {
    tokenSvc     TokenService
    isProduction bool
    cookieDomain string
}

type AuthConfig struct {
    IsProduction bool
    TokenSvc     TokenService
    CookieDomain string
}

func NewAuthMiddleware(cfg AuthConfig) *AuthMiddleware {
    return &AuthMiddleware{
        tokenSvc:     cfg.TokenSvc,
        isProduction: cfg.IsProduction,
        cookieDomain: cfg.CookieDomain,
    }
}

func (m *AuthMiddleware) Handle(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c *echo.Context) error {
        // Extract token from cookie
        accessCookie := "__Secure-access_token"
        if !m.isProduction { accessCookie = "access_token" }
        cookie, err := c.Cookie(accessCookie)
        if err != nil {
            return echo.NewHTTPError(401, "unauthorized")
        }
        // Validate token
        claims, err := m.tokenSvc.Validate(c.Request().Context(), cookie.Value)
        if err != nil {
            return echo.NewHTTPError(401, "invalid token")
        }
        c.Set("claims", claims)
        return next(c)
    }
}
```

### Template: Rate Limit Middleware (four-tier)
```go
func RateLimitMiddleware(rl *RateLimiter, identifyUser func(c *echo.Context) (string, bool)) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c *echo.Context) error {
            var result RateLimitResult
            var err error
            // Tier 1: Authenticated
            if userID, ok := identifyUser(c); ok {
                result, err = rl.AuthenticatedAllow(c.Request().Context(), userID)
            } else {
                // Tier 2: Anonymous (cookie)
                if anonCookie, cookieErr := c.Cookie("__Secure-anon_token"); cookieErr == nil {
                    result, err = rl.AnonymousAllow(c.Request().Context(), anonCookie.Value)
                } else {
                    // Tier 3: Global (IP)
                    result, err = rl.GlobalAllow(c.Request().Context(), c.RealIP())
                }
            }
            if err != nil || !result.Allowed {
                return echo.NewHTTPError(429, "rate limit exceeded")
            }
            return next(c)
        }
    }
}
```

### Template: Security Headers Middleware (factory-based)
```go
func SecurityHeaders() echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c *echo.Context) error {
            c.Response().Header().Set("Content-Security-Policy", "default-src 'self'")
            c.Response().Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
            c.Response().Header().Set("X-Frame-Options", "DENY")
            c.Response().Header().Set("X-Content-Type-Options", "nosniff")
            c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
            return next(c)
        }
    }
}
```

## Uses Skills
| Skill | When |
|-------|------|
| `echo` | Always loaded — Echo v5 patterns, *echo.Context, echo.MiddlewareFunc |
| `go` | Always loaded — Go 1.26 patterns |

## Verification
```bash
# 1. Compile check
go build ./... && echo "PASS: compiles"

# 2. Check echo.MiddlewareFunc return type
grep 'echo.MiddlewareFunc' {{.FilePath}} || echo "WARN: not returning echo.MiddlewareFunc"

# 3. Check security headers
grep 'X-Frame-Options.*DENY' {{.FilePath}} || echo "WARN: missing X-Frame-Options"

# 4. Check no global state
grep 'var ' {{.FilePath}} | grep -v 'Err' && echo "WARN: potential global state" || echo "PASS"

# 5. Check cookie-based auth (not header)
grep -i 'authorization' {{.FilePath}} && echo "WARN: should use cookies, not Authorization header" || echo "PASS"
```
