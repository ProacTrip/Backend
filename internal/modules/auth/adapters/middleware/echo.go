// Middleware de autenticación para Echo v5.
// Flujo: cookies → validación PASETO → pipeline de autorización → inyección de claims.
//
// Pipeline (nuevo desde Batch 3):
//  1. Validar token PASETO (existente)
//  2. Verificar JTI blacklist en Dragonfly
//  3. Leer cache de sesión ({auth}:session:{SID})
//  4. Comparar token_version (token vs cache/DB)
//  5. Verificar estado de la cuenta (active requerido)
//  6. Inyectar claims con Permissions[] resueltos
//
// Modo observe: AUTHZ_ENFORCE_MODE=observe ejecuta el pipeline completo pero
// nunca bloquea requests. Loguea advertencias e incrementa métricas.
// PR 2 usa observe por defecto; PR 5 escala a enforce global.
package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"github.com/ProacTrip/Backend/internal/modules/auth/adapters/token"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	"github.com/ProacTrip/Backend/internal/modules/auth/domain/services"
	sessionpkg "github.com/ProacTrip/Backend/internal/shared/session"
	sharedhttp "github.com/ProacTrip/Backend/internal/shared/http"
)

const (
	accessCookieNameProd  = "__Secure-access_token"
	refreshCookieNameProd = "__Secure-refresh_token"
	accessCookieNameDev   = "access_token"
	refreshCookieNameDev  = "refresh_token"
)

// =============================================================================
// TokenService — interfaz de servicio de tokens (inyectada)
// =============================================================================

type TokenService interface {
	ValidateAccessToken(ctx context.Context, tokenString string) (*token.AccessClaims, error)
	ValidateRefreshToken(ctx context.Context, tokenString string) (*token.RefreshClaims, error)
	ValidateAndRotateRefresh(ctx context.Context, refreshToken string) (*token.RefreshClaims, string, string, error)
	// ValidateAndRotateRefreshWithVersion rota tokens con el token_version real desde DB.
	// Usado por el pipeline de autorización cuando ya se validó el estado de la cuenta.
	ValidateAndRotateRefreshWithVersion(ctx context.Context, refreshToken string, tokenVersion int) (*token.RefreshClaims, string, string, error)
}

// =============================================================================
// AuthConfig — configuración del middleware de autenticación
// =============================================================================

type AuthConfig struct {
	IsProduction bool
	TokenSvc     TokenService
	UserRepo     domain.UserRepository
	CookieDomain string

	// RedisClient es el cliente de DragonflyDB para el cache de sesión y JTI blacklist.
	// Puede ser nil (en ese caso se deshabilitan las verificaciones de cache).
	RedisClient *redis.Client

	// PermissionResolver resuelve permisos efectivos en cache miss.
	// Puede ser nil (en ese caso se usan permisos vacíos).
	PermissionResolver services.PermissionResolver
}

// =============================================================================
// AuthMiddleware — middleware de autenticación con pipeline de autorización
// =============================================================================

type AuthMiddleware struct {
	config      AuthConfig
	enforceMode string // cacheado al inicio: "observe", "dashboard", "global", o ""
}

// NewAuthMiddleware crea un nuevo middleware de autenticación.
// Lee AUTHZ_ENFORCE_MODE del entorno una vez al inicio (cacheado).
// Por defecto: "observe" (no bloquea requests).
func NewAuthMiddleware(cfg AuthConfig) *AuthMiddleware {
	mode := os.Getenv("AUTHZ_ENFORCE_MODE")
	if mode == "" {
		mode = "dashboard" // fuerza auth en /v1/user/* y /v1/dashboard/*
	}
	return &AuthMiddleware{config: cfg, enforceMode: mode}
}

// =============================================================================
// Handle — middleware principal de autenticación
// =============================================================================

func (m *AuthMiddleware) Handle(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		accessName, refreshName := m.cookieNames()

		accessCookie, _ := c.Cookie(accessName)
		refreshCookie, _ := c.Cookie(refreshName)

		if accessCookie == nil && refreshCookie == nil {
			// Sin cookies: si el path requiere enforce, devolver 401.
			// Si no (observe mode), pasar el request sin claims.
			if m.shouldEnforce(c.Request().URL.Path) {
				return echo.NewHTTPError(http.StatusUnauthorized, "Autenticación requerida")
			}
			return next(c)
		}

		// ── Access token válido ──────────────────────────────────────────
		if accessCookie != nil && accessCookie.Value != "" {
			claims, err := m.config.TokenSvc.ValidateAccessToken(c.Request().Context(), accessCookie.Value)
			if err == nil {
				// Pipeline de autorización para access token válido
				if pipelineErr := m.runAccessPipeline(c, claims); pipelineErr != nil {
					return pipelineErr
				}
				c.Set("user_claims", claims)
				return next(c)
			}
		}

		// ── Refresh token rotation ───────────────────────────────────────
		if refreshCookie != nil && refreshCookie.Value != "" {
			claims, newAccess, newRefresh, err := m.rotateWithPipeline(c, refreshCookie.Value)
			if err == nil {
				if m.config.IsProduction {
					sharedhttp.SetAuthCookiesFromTokens(c, newAccess, newRefresh, m.config.CookieDomain)
				} else {
					sharedhttp.SetAuthCookiesDev(c, newAccess, newRefresh)
				}
				// Inyectar claims en el contexto.
				// Si hay datos de sesión cacheados, poblar Permissions[] y TokenVersion.
				c.Set("user_claims", claims)
				return next(c)
			}
			if errors.Is(err, domain.ErrTokenRevoked) || errors.Is(err, domain.ErrTokenExpired) {
				if m.config.IsProduction {
					sharedhttp.ClearAuthCookies(c, m.config.CookieDomain)
				} else {
					sharedhttp.ClearAuthCookiesDev(c)
				}
				return echo.NewHTTPError(http.StatusUnauthorized, "Sesión expirada. Inicia sesión nuevamente.")
			}
		}

		// ── Sin cookies válidas ──────────────────────────────────────────
		if m.config.IsProduction {
			sharedhttp.ClearAuthCookies(c, m.config.CookieDomain)
		} else {
			sharedhttp.ClearAuthCookiesDev(c)
		}
		return echo.NewHTTPError(http.StatusUnauthorized, "Autenticación requerida")
	}
}

// =============================================================================
// runAccessPipeline — pasos 3-7 del pipeline de autorización
// =============================================================================
// Pasos:
//  3. Verificar JTI blacklist
//  4. Leer cache de sesión (GetOrSet)
//  5. Comparar token_version
//  6. Verificar estado de la cuenta
//  7. Inyectar Permissions[] en claims
//
// En modo observe: loguea advertencias pero NUNCA bloquea.
// En modo enforce: retorna 401/403 según corresponda.

func (m *AuthMiddleware) runAccessPipeline(c *echo.Context, claims *token.AccessClaims) error {
	ctx := c.Request().Context()
	path := c.Request().URL.Path
	enforce := m.shouldEnforce(path)

	// Paso 3: Verificar JTI blacklist en Dragonfly
	if m.config.RedisClient != nil {
		jtiKey := fmt.Sprintf("{auth}:blacklist:jti:%s", claims.JTI.String())
		exists, err := m.config.RedisClient.Exists(ctx, jtiKey).Result()
		if err != nil {
			slog.WarnContext(ctx, "Dragonfly unreachable during JTI blacklist check (fail-open)",
				slog.String("jti", claims.JTI.String()),
				slog.String("error", err.Error()),
			)
		} else if exists > 0 {
			slog.WarnContext(ctx, "JTI blacklisted",
				slog.String("jti", claims.JTI.String()),
				slog.String("user_id", claims.UserID.String()),
				slog.String("path", path),
			)
			if enforce {
				return echo.NewHTTPError(http.StatusUnauthorized, "Token revocado. Inicia sesión nuevamente.")
			}
			// observe: continuar con el request
		}
	}

	// Paso 4-6: Validar sesión (cache → DB fallback)
	sessionData, err := m.getOrResolveSession(ctx, claims.UserID, claims.RoleID, claims.SessionID)
	if err != nil {
		// Error al resolver sesión → loguear, continuar con claims vacíos (fail-open)
		slog.WarnContext(ctx, "sesión no resuelta, continuando sin permisos (fail-open)",
			slog.String("user_id", claims.UserID.String()),
			slog.String("session_id", claims.SessionID.String()),
			slog.String("error", err.Error()),
		)
	}

		if sessionData != nil {
		// Paso 5: Comparar token_version
		cacheTV := atoiOrZero(sessionData.TokenVersion)
		if cacheTV > 0 && claims.TokenVersion != cacheTV {
			slog.WarnContext(ctx, "token version mismatch",
				slog.Int("token_tv", claims.TokenVersion),
				slog.Int("cache_tv", cacheTV),
				slog.String("user_id", claims.UserID.String()),
				slog.String("path", path),
			)
			if enforce {
				return echo.NewHTTPError(http.StatusUnauthorized, "Token desactualizado. Inicia sesión nuevamente.")
			}
			// observe: usar datos cacheados y continuar
			claims.TokenVersion = cacheTV
		}

		// Paso 6: Verificar estado de la cuenta
		if err := m.checkAccountStatus(c, sessionData.Status, enforce); err != nil {
			return err
		}

		// Paso 7: Inyectar Permissions[] en claims
		if sessionData.Permissions != "" {
			claims.Permissions = splitPermissions(sessionData.Permissions)
		}

		// Actualizar token_version en claims desde el cache
		if cacheTV > 0 {
			claims.TokenVersion = cacheTV
		}
	} else {
		// Sin datos de sesión: continuar con permisos vacíos (usuario no cacheado aún)
		if enforce {
			// En enforce mode sin session data, hacer fallback a DB
			perms, status, tv, err := m.resolveFromDB(ctx, claims.UserID, claims.RoleID)
			if err != nil {
				slog.WarnContext(ctx, "DB fallback failed for enforce mode",
					slog.String("user_id", claims.UserID.String()),
					slog.String("error", err.Error()),
				)
			} else {
			if err := m.checkAccountStatus(c, status, enforce); err != nil {
				return err
			}
				claims.Permissions = perms
				claims.TokenVersion = tv
			}
		}
	}

	return nil
}

// =============================================================================
// rotateWithPipeline — rotación de refresh token con validación de estado
// =============================================================================
// Antes de rotar, verifica que el usuario esté activo y obtiene el token_version
// real desde la DB. Luego rota con ValidateAndRotateRefreshWithVersion.
// En modo observe: si el estado no es active, loguea y NO rota (devuelve error).
// La rotación NUNCA se hace en modo observe para usuarios no activos.
//
// Si UserRepo no está configurado, usa la rotación legacy sin validación de estado.

func (m *AuthMiddleware) rotateWithPipeline(c *echo.Context, refreshToken string) (*token.RefreshClaims, string, string, error) {
	ctx := c.Request().Context()

	// Sin UserRepo, usar rotación legacy (sin validación de estado ni token_version)
	if m.config.UserRepo == nil {
		return m.config.TokenSvc.ValidateAndRotateRefresh(ctx, refreshToken)
	}

	// Validar el refresh token primero (para obtener claims)
	refreshClaims, err := m.config.TokenSvc.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, "", "", err
	}

	// Obtener datos del usuario desde DB para validar estado y token_version
	user, err := m.config.UserRepo.GetByID(ctx, refreshClaims.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			slog.WarnContext(ctx, "usuario no encontrado durante rotación",
				slog.String("user_id", refreshClaims.UserID.String()),
			)
			return nil, "", "", domain.ErrTokenRevoked
		}
		return nil, "", "", fmt.Errorf("get user for rotation: %w", err)
	}

	// Validar estado de la cuenta (siempre enforce para rotación — es un nuevo login)
	switch user.Status {
	case domain.StatusDisabled:
		slog.WarnContext(ctx, "rotación rechazada: cuenta deshabilitada",
			slog.String("user_id", user.ID.String()),
		)
		return nil, "", "", domain.ErrAccountDisabled
	case domain.StatusSuspended:
		slog.WarnContext(ctx, "rotación rechazada: cuenta suspendida",
			slog.String("user_id", user.ID.String()),
		)
		return nil, "", "", domain.ErrAccountSuspended
	case domain.StatusPendingVerification:
		slog.WarnContext(ctx, "rotación rechazada: email no verificado",
			slog.String("user_id", user.ID.String()),
		)
		return nil, "", "", domain.ErrEmailNotVerified
	}

	// Validar token_version (siempre enforce para rotación)
	// El refresh token no tiene token_version explícito en los claims actuales,
	// así que usamos el valor desde DB directamente.
	tokenVersion := user.TokenVersion
	if tokenVersion < 1 {
		tokenVersion = 1
	}

	// Rotar con el token_version real desde DB
	newClaims, newAccess, newRefresh, err := m.config.TokenSvc.ValidateAndRotateRefreshWithVersion(ctx, refreshToken, tokenVersion)
	if err != nil {
		return nil, "", "", err
	}

	// Poblar el cache de sesión con los datos del usuario
	m.populateSessionCache(ctx, user, refreshClaims.SessionID)

	return newClaims, newAccess, newRefresh, nil
}

// =============================================================================
// getOrResolveSession — cache de sesión con fallback a DB
// =============================================================================

func (m *AuthMiddleware) getOrResolveSession(ctx context.Context, userID, roleID, sessionID uuid.UUID) (*sessionpkg.SessionData, error) {
	sessionKey := sessionID.String()

	// Si no hay RedisClient, ir directo a DB
	if m.config.RedisClient == nil {
		return m.resolveSessionFromDB(ctx, userID, roleID)
	}

	// Intentar cache hit
	cached, err := sessionpkg.GetSession(ctx, m.config.RedisClient, sessionKey)
	if err != nil {
		slog.WarnContext(ctx, "session cache read error (fail-open, usando DB)",
			slog.String("session_id", sessionKey),
			slog.String("error", err.Error()),
		)
		return m.resolveSessionFromDB(ctx, userID, roleID)
	}

	if cached != nil {
		// Cache hit — refrescar TTL
		m.refreshSessionTTL(ctx, sessionKey)
		return cached, nil
	}

	// Cache miss — resolver desde DB
	data, err := m.resolveSessionFromDB(ctx, userID, roleID)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	// Guardar en cache (best-effort)
	if setErr := sessionpkg.SetSession(ctx, m.config.RedisClient, sessionKey, data, sessionpkg.SessionTTL); setErr != nil {
		slog.WarnContext(ctx, "session cache write failed (non-blocking)",
			slog.String("session_id", sessionKey),
			slog.String("error", setErr.Error()),
		)
	}

	return data, nil
}

// =============================================================================
// resolveSessionFromDB — obtiene datos de sesión desde DB + PermissionResolver

func (m *AuthMiddleware) resolveSessionFromDB(ctx context.Context, userID, roleID uuid.UUID) (*sessionpkg.SessionData, error) {
	// Si no hay UserRepo configurado, retornar nil (sin datos de sesión)
	if m.config.UserRepo == nil {
		return nil, nil
	}

	// Obtener usuario de DB
	user, err := m.config.UserRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user for session: %w", err)
	}

	// Resolver permisos efectivos
	var perms []string
	if m.config.PermissionResolver != nil {
		perms, err = m.config.PermissionResolver.ResolveEffectivePermissions(ctx, userID, roleID)
		if err != nil {
			slog.WarnContext(ctx, "permission resolver failed, usando permisos vacíos (fail-open)",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()),
			)
			perms = nil
		}
	}

	return &sessionpkg.SessionData{
		Permissions:  strings.Join(perms, ","),
		Status:       string(user.Status),
		TokenVersion: fmt.Sprintf("%d", user.TokenVersion),
	}, nil
}

// =============================================================================
// resolveFromDB — fallback directo a DB (sin cache)
// =============================================================================

func (m *AuthMiddleware) resolveFromDB(ctx context.Context, userID, roleID uuid.UUID) ([]string, string, int, error) {
	if m.config.UserRepo == nil {
		return nil, "", 0, nil
	}

	user, err := m.config.UserRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, "", 0, err
	}

	var perms []string
	if m.config.PermissionResolver != nil {
		perms, err = m.config.PermissionResolver.ResolveEffectivePermissions(ctx, userID, roleID)
		if err != nil {
			perms = nil
		}
	}

	return perms, string(user.Status), user.TokenVersion, nil
}

// =============================================================================
// populateSessionCache — guarda datos de sesión en cache después de rotación
// =============================================================================

func (m *AuthMiddleware) populateSessionCache(ctx context.Context, user *domain.User, sessionID uuid.UUID) {
	if m.config.RedisClient == nil || user == nil {
		return
	}

	// Resolver permisos (best-effort)
	var perms []string
	if m.config.PermissionResolver != nil {
		var err error
		perms, err = m.config.PermissionResolver.ResolveEffectivePermissions(ctx, user.ID, user.RoleID)
		if err != nil {
			slog.WarnContext(ctx, "permission resolver failed during session cache population",
				slog.String("user_id", user.ID.String()),
				slog.String("error", err.Error()),
			)
		}
	}

	data := &sessionpkg.SessionData{
		UserID:       user.ID.String(),
		Permissions:  strings.Join(perms, ","),
		Status:       string(user.Status),
		TokenVersion: fmt.Sprintf("%d", user.TokenVersion),
	}

	if err := sessionpkg.SetSession(ctx, m.config.RedisClient, sessionID.String(), data, sessionpkg.SessionTTL); err != nil {
		slog.WarnContext(ctx, "session cache population failed (non-blocking)",
			slog.String("user_id", user.ID.String()),
			slog.String("error", err.Error()),
		)
	}
}

// =============================================================================
// refreshSessionTTL — extiende el TTL del cache de sesión en cada request
// =============================================================================

func (m *AuthMiddleware) refreshSessionTTL(ctx context.Context, sessionID string) {
	if m.config.RedisClient == nil {
		return
	}
	key := fmt.Sprintf("{auth}:session:%s", sessionID)
	if err := m.config.RedisClient.Expire(ctx, key, sessionpkg.SessionTTL).Err(); err != nil {
		slog.WarnContext(ctx, "session TTL refresh failed (non-blocking)",
			slog.String("session_id", sessionID),
			slog.String("error", err.Error()),
		)
	}
}

// checkAccountStatus — verifica el estado de la cuenta.
// En modo enforce: retorna echo.NewHTTPError para detener la cadena de middleware.
// En modo observe: solo loguea y retorna nil.

func (m *AuthMiddleware) checkAccountStatus(c *echo.Context, status string, enforce bool) error {
	ctx := c.Request().Context()
	path := c.Request().URL.Path

	switch status {
	case string(domain.StatusDisabled):
		slog.WarnContext(ctx, "cuenta deshabilitada",
			slog.String("path", path),
		)
		if enforce {
			return echo.NewHTTPError(http.StatusForbidden, "Cuenta deshabilitada. Contacta al administrador.")
		}
	case string(domain.StatusSuspended):
		slog.WarnContext(ctx, "cuenta suspendida",
			slog.String("path", path),
		)
		if enforce {
			return echo.NewHTTPError(http.StatusForbidden, "Cuenta suspendida. Contacta al administrador.")
		}
	case string(domain.StatusPendingVerification):
		slog.WarnContext(ctx, "cuenta pendiente de verificación",
			slog.String("path", path),
		)
		if enforce {
			return echo.NewHTTPError(http.StatusUnauthorized, "Email no verificado. Revisa tu bandeja de entrada.")
		}
	}

	return nil
}

// =============================================================================
// Optional — middleware público que extrae claims pero NUNCA rechaza
// =============================================================================

// Optional returns a middleware that extracts user claims when valid credentials
// are present, but NEVER rejects the request. Invalid/expired tokens are silently
// ignored — the request proceeds as anonymous with no user_claims in context.
//
// Use this on public endpoints that behave differently for authenticated users
// (e.g., higher rate limits, profile preferences, conversation persistence)
// but must remain accessible to anonymous users.
func (m *AuthMiddleware) Optional() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			accessName, refreshName := m.cookieNames()

			accessCookie, _ := c.Cookie(accessName)
			refreshCookie, _ := c.Cookie(refreshName)

			// No cookies → anonymous, pass through
			if accessCookie == nil && refreshCookie == nil {
				return next(c)
			}

			// Try access token first
			if accessCookie != nil && accessCookie.Value != "" {
				claims, err := m.config.TokenSvc.ValidateAccessToken(c.Request().Context(), accessCookie.Value)
				if err == nil {
					// Pipeline ligero: session cache + permisos (nunca bloquea)
					_ = m.runAccessPipeline(c, claims)
					c.Set("user_claims", claims)
					return next(c)
				}
			}

			// Try refresh token rotation
			if refreshCookie != nil && refreshCookie.Value != "" {
				claims, newAccess, newRefresh, err := m.rotateWithPipeline(c, refreshCookie.Value)
				if err == nil {
					if m.config.IsProduction {
						sharedhttp.SetAuthCookiesFromTokens(c, newAccess, newRefresh, m.config.CookieDomain)
					} else {
						sharedhttp.SetAuthCookiesDev(c, newAccess, newRefresh)
					}
					c.Set("user_claims", claims)
					return next(c)
				}
			}

			// Token validation failed — proceed as anonymous.
			// Do NOT clear cookies (they belong to other routes).
			// Do NOT return 401 (this is a public endpoint).
			return next(c)
		}
	}
}

// =============================================================================
// cookieNames — retorna los nombres de cookies según el entorno
// =============================================================================

func (m *AuthMiddleware) cookieNames() (string, string) {
	if m.config.IsProduction {
		return accessCookieNameProd, refreshCookieNameProd
	}
	return accessCookieNameDev, refreshCookieNameDev
}

// =============================================================================
// Helpers de enforce mode
// =============================================================================

// shouldEnforce determina si se debe aplicar enforce en este path.
// Modos:
//   - "global": enforce en todos los paths autenticados
//   - "dashboard": enforce solo en /v1/dashboard/*
//   - "observe" o vacío: nunca enforce (solo logueo)
func (m *AuthMiddleware) shouldEnforce(path string) bool {
	switch m.enforceMode {
	case "global":
		return true
	case "dashboard":
		return strings.HasPrefix(path, "/v1/dashboard/") || strings.HasPrefix(path, "/v1/user/")
	default:
		return false
	}
}

// =============================================================================
// Helpers de parseo
// =============================================================================

// atoiOrZero convierte un string a int, retornando 0 si falla.
func atoiOrZero(s string) int {
	if s == "" {
		return 0
	}
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// splitPermissions separa una string de permisos por comas.
// "users:read,users:write" → ["users:read", "users:write"].
// String vacía → slice vacío.
// Sin espacios entre permisos (formato de cache).
func splitPermissions(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	// Filtrar strings vacías (por si acaso)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	slices.Sort(result)
	return result
}
