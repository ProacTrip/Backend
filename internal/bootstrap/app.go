package bootstrap

// App bootrapea la aplicación: inicializa Echo, DB, Redis, módulos y rutas.
// Gestiona el ciclo de vida completo del servidor.

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ProacTrip/Backend/internal/config"
	authModule "github.com/ProacTrip/Backend/internal/modules/auth"
	authmiddleware "github.com/ProacTrip/Backend/internal/modules/auth/adapters/middleware"
	environmentModule "github.com/ProacTrip/Backend/internal/modules/environment"
	envShared "github.com/ProacTrip/Backend/internal/modules/environment/features/shared"
	notifModule "github.com/ProacTrip/Backend/internal/modules/notification"
	searchModule "github.com/ProacTrip/Backend/internal/modules/search"
	searchDomain "github.com/ProacTrip/Backend/internal/modules/search/domain"
	ai_deepseek "github.com/ProacTrip/Backend/internal/modules/search/adapters/ai/deepseek"
	ai_ollama "github.com/ProacTrip/Backend/internal/modules/search/adapters/ai/ollama"
	searchConv "github.com/ProacTrip/Backend/internal/modules/search/shared/conversation"
	searchShared "github.com/ProacTrip/Backend/internal/modules/search/features/shared"
	userModule "github.com/ProacTrip/Backend/internal/modules/user"
	userAdapters "github.com/ProacTrip/Backend/internal/modules/user/adapters"
	"github.com/ProacTrip/Backend/internal/modules/auth/features/register"
	"github.com/ProacTrip/Backend/internal/shared/cache"
	contextutil "github.com/ProacTrip/Backend/internal/shared/context"
	"github.com/ProacTrip/Backend/internal/shared/database"
	sharederrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	sharedhttp "github.com/ProacTrip/Backend/internal/shared/http"
	sharedmiddleware "github.com/ProacTrip/Backend/internal/shared/middleware"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/redis/go-redis/v9"
)

// StartConfig es un alias para echo.StartConfig
type StartConfig = echo.StartConfig

// Compile-time interface check: EnvironmentResolverAdapter implements register.EnvironmentResolver.
var _ register.EnvironmentResolver = (*envShared.EnvironmentResolverAdapter)(nil)

type App struct {
	Echo        *echo.Echo
	RedisClient *redis.Client
	EventBus    *eventbus.EventBus
	Cfg         *config.Config
	DB          *database.PoolManager // Pool manager con DB por módulo
	startTime   time.Time             // server start time for health checks

	// Modules
	AuthModule         *authModule.Module
	UserModule         *userModule.Module
	NotificationModule *notifModule.Module
	SearchModule       *searchModule.Module
	EnvironmentModule  *environmentModule.Module

	// Lifecycle
	appCancel context.CancelFunc // cancels background consumers on shutdown
}

func NewApp(cfg *config.Config, logger *slog.Logger) (*App, error) {
	e := echo.New()
	e.Logger = logger

	// Echo v5.1.0: IP extraction segura con X-Forwarded-For.
	// Solo confía en loopback y rangos privados (Cloudflare re-escribe XFF).
	e.IPExtractor = echo.ExtractIPFromXFFHeader(
		echo.TrustLoopback(true),
		echo.TrustPrivateNet(true),
	)

	// Middleware: request ID, traceparent, logging, recovery, CORS
	e.Use(middleware.RequestID())

	// Traceparent W3C header - generar trace ID si no existe
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			traceparent := c.Request().Header.Get("traceparent")
			if traceparent != "" {
				c.Set("traceparent", traceparent)
			} else {
				traceID := contextutil.NewTraceID()
				c.Response().Header().Set("traceparent", traceID.Traceparent())
				c.Response().Header().Set("X-Trace-Id", traceID.TraceID)
			}
			return next(c)
		}
	})

	// Store trace ID / request ID in context.Context for downstream use
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			traceID := c.Response().Header().Get(echo.HeaderXRequestID)
			if traceID == "" {
				traceID = c.Request().Header.Get("X-Trace-Id")
			}
			if traceID != "" {
				ctx := contextutil.WithTraceID(c.Request().Context(), traceID)
				c.SetRequest(c.Request().WithContext(ctx))
			}
			return next(c)
		}
	})

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		Skipper: func(c *echo.Context) bool {
			path := c.Request().URL.Path
			return path == "/health" || path == "/ready"
		},
		LogStatus: true,
		LogURI:    true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			logger.Info("request",
				"uri", v.URI,
				"status", v.Status,
				"latency", v.Latency,
			)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{cfg.Frontend.GetURL()},
		AllowCredentials: true,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"X-Request-Id",
			"Idempotency-Key",
			"X-Trace-Id",
		},
		MaxAge: 86400,
	}))

	// Security headers (CSP, HSTS, X-Frame-Options, etc.) — required by all API docs
	e.Use(sharedmiddleware.SecurityHeaders())

	// HTTPErrorHandler: errores RFC 9457
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		// Verificar si la respuesta ya fue enviada
		if resp, _ := echo.UnwrapResponse(c.Response()); resp != nil && resp.Committed {
			return
		}

		if he, ok := errors.AsType[*echo.HTTPError](err); ok {
			traceID := sharedhttp.GetOrGenerateTraceID(c)
			instance := c.Request().URL.Path

			problem := sharedhttp.MapHTTPErrorToProblem(he, instance, traceID)

			_ = c.JSON(he.Code, problem)
			return
		}

		// Otros errores → verificar si ya es un Problem
		if prob, ok := err.(*sharederrors.Problem); ok {
			_ = c.JSON(prob.Status, prob)
			return
		}

		// Error desconocido → RFC 9457
		problem := sharederrors.ErrInternalError(err.Error(), err).WithInstance(c.Request().URL.Path)
		_ = c.JSON(http.StatusInternalServerError, problem)
	}

	// Database: pool por módulo
	poolMgr := database.NewPoolManager(database.PoolConfig{
		Auth:            cfg.DB.GetDSNForDB("proactrip_auth"),
		User:            cfg.DB.GetDSNForDB("proactrip_user"),
		Notification:    cfg.DB.GetDSNForDB("proactrip_notification"),
		Search:          cfg.DB.GetDSNForDB("proactrip_search"),
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 10 * time.Minute,
	})

	// Obtener pools para cada módulo
	authPool, err := poolMgr.GetPool(database.DBAuth)
	if err != nil {
		return nil, fmt.Errorf("auth DB pool: %w", err)
	}
	userPool, err := poolMgr.GetPool(database.DBUser)
	if err != nil {
		return nil, fmt.Errorf("user DB pool: %w", err)
	}
	notifPool, err := poolMgr.GetPool(database.DBNotification)
	if err != nil {
		return nil, fmt.Errorf("notification DB pool: %w", err)
	}
	searchPool, err := poolMgr.GetPool(database.DBSearch)
	if err != nil {
		return nil, fmt.Errorf("search DB pool: %w", err)
	}

	// Redis / Dragonfly
	dragonflyCfg := cache.DefaultConfig(cfg.Dragonfly.Addr(), cfg.Dragonfly.Password)
	df, err := cache.NewDragonfly(dragonflyCfg)
	if err != nil {
		return nil, fmt.Errorf("dragonfly: %w", err)
	}
	rdb := df.Client()

	// Rate Limiter
	rateLimiter := ratelimit.NewRateLimiter(rdb, cfg.RateLimit)

	// Global Rate Limiting (DDoS)
	e.Use(ratelimit.GlobalRateLimitMiddleware(rateLimiter))

	ctx := context.Background()
	appCtx, appCancel := context.WithCancel(ctx)
	var cancelOnError = true
	defer func() {
		if cancelOnError {
			appCancel()
		}
	}()

	// Event Bus (Streams)
	eventBus := eventbus.NewEventBus(rdb)

	// Inicializar módulos

	// Environment Module (IP geolocation + weather)
	// Created BEFORE auth because auth registration needs the EnvironmentResolver.
	environmentMod := environmentModule.NewModule(environmentModule.Config{
		OpenWeatherAPIKey:   cfg.Environment.OpenWeatherAPIKey,
		OpenWeatherCacheTTL: cfg.Environment.OpenWeatherCacheTTL,
		IpQueryBaseURL:      cfg.Environment.IpQueryBaseURL,
		Cache:               df,
		RateLimiter:         rateLimiter,
	})

	// Auth Module (incluye DragonflyClient para idempotency)
	authMod, err := authModule.NewModule(authModule.Config{
		EnvResolver: environmentMod.EnvironmentResolver,
		PostgresPool:         authPool,
		DragonflyClient:      rdb,
		PasetoKey:            cfg.PasetoKeyBytes, // Bytes decodificados de hex
		AccessTokenTTL:       15 * time.Minute,
		RefreshTokenTTL:      7 * 24 * time.Hour,
		EmailVerificationTTL: 24 * time.Hour,
		PasswordResetTTL:     1 * time.Hour,
		OAuthConfig:          cfg.OAuth,
		FrontendURL:          cfg.Frontend.GetURL(),
		IsProduction:         cfg.Server.Env == "production",
		CookieDomain:         cfg.CookieDomain,
		EventPublisher:       eventBus,
	})
	if err != nil {
		return nil, err
	}

	// User Module (simplified for MVP)
	encKeyBytes, _ := cfg.Medical.EncryptionKeyBytes()
	userMod, err := userModule.NewModule(userModule.Config{
		PostgresPool:  userPool,
		RedisClient:   rdb,
		EventBus:      eventBus,
		EncryptionKey: encKeyBytes,
	})
	if err != nil {
		return nil, err
	}

	// Notification Module (simplified for MVP)
	notifMod, err := notifModule.NewModule(notifModule.Config{
		PostgresPool:   notifPool,
		RedisClient:    rdb,
		ResendAPIKey:   cfg.Email.ResendAPIKey,
		FrontendConfig: cfg.Frontend,
		RateLimiter:    rateLimiter,
	})
	if err != nil {
		return nil, err
	}

	// Start notification consumer (BACKGROUND - consume eventos de Dragonfly Streams)
	if err := notifMod.EventConsumer.Start(appCtx); err != nil {
		slog.Warn("notification consumer failed to start", "error", err)
		// No es fatal - el servidor puede iniciar sin el consumer
	}

	// Wire resend-verification feature (notification module must exist first)
	authMod.WireResendVerification(notifMod.SendVerificationEmailUseCase)

	// Start user consumer (BACKGROUND - consume eventos de Dragonfly Streams)
	if err := userMod.EventConsumer.Start(appCtx); err != nil {
		slog.Warn("user event consumer failed to start", "error", err)
		// No es fatal - el servidor puede iniciar sin el consumer
	}

	// Start avatar validator pipeline (BACKGROUND) — Phase 4
	if err := userMod.Start(appCtx); err != nil {
		slog.Warn("user avatar validator failed to start", "error", err)
		// No es fatal - el servidor puede iniciar sin el validator
	}

	// Search Module — create AI interpreter if configured
	var aiInterpreter searchDomain.AIInterpreter
	if cfg.AI.Provider == "deepseek" && cfg.AI.APIKey != "" {
		baseURL := cmp.Or(cfg.AI.BaseURL, "https://api.deepseek.com/v1")
		model := cmp.Or(cfg.AI.Model, "deepseek-chat")
		client := ai_deepseek.NewClient(cfg.AI.APIKey, cfg.AI.Timeout,
			ai_deepseek.WithBaseURL(baseURL),
			ai_deepseek.WithModel(model),
		)
		aiInterpreter = ai_deepseek.NewAdapter(client)
		slog.Info("AI interpreter: deepseek configured", slog.String("model", model))
	} else if cfg.AI.Provider == "ollama" {
		baseURL := cmp.Or(cfg.AI.BaseURL, "http://localhost:11434/v1")
		model := cmp.Or(cfg.AI.Model, "llama3.2")
		var opts []ai_ollama.ClientOpt
		if cfg.AI.BaseURL != "" {
			opts = append(opts, ai_ollama.WithBaseURL(baseURL))
		}
		if cfg.AI.Model != "" {
			opts = append(opts, ai_ollama.WithModel(model))
		}
		client := ai_ollama.NewClient(cfg.AI.Timeout, opts...)
		aiInterpreter = ai_ollama.NewAdapter(client)
		slog.Info("AI interpreter: ollama configured", slog.String("model", model))
	}

	// Conversation PG store for auth users
	convPgStore := searchConv.NewPgConversationStore(searchPool)

	// SavedSearchResolver bridges user module's SavedSearchRepo to search module's SavedSearchProvider.
	savedSearchResolver := userAdapters.NewSearchResolver(userMod.SavedSearchRepo)

	searchMod, err := searchModule.NewModule(searchModule.Config{
		Provider:          nil, // created from SerpAPIKey/SerpAPITimeout
		SerpAPIKey:        cfg.SerpAPIKey,
		SerpAPITimeout:    30 * time.Second,
		SearchCache:       df,
		DetailsCache:      df,
		HotelSearchCache:  df,
		HotelDetailsCache: df,
		Repo:              nil, // created from PgxPool
		PgxPool:           searchPool,
		SearchTTL:         15 * time.Minute,
		FlightDetailsTTL:  15 * time.Minute,
		HotelSearchTTL:    5 * time.Minute,
		HotelDetailsTTL:   15 * time.Minute,
		RateLimiter:       rateLimiter,
		RedisClient:       rdb,
		SearchDefaults: searchShared.SearchDefaultConfig{
			Currency:    cfg.DefaultCurrency,
			Language:    cfg.DefaultLanguage,
			CountryCode: cfg.DefaultCountryCode,
		},
		AIInterpreter:     aiInterpreter,
		ConversationStore: convPgStore,
		EventBus:          eventBus,
		SavedSearchProvider: savedSearchResolver,
	})
	if err != nil {
		return nil, err
	}

	// Auth Middleware (silent refresh token rotation)
	authMiddleware := authmiddleware.NewAuthMiddleware(authmiddleware.AuthConfig{
		IsProduction: cfg.Server.Env == "production",
		TokenSvc:     authMod.TokenService,
		UserRepo:     authMod.Repository,
		CookieDomain: cfg.CookieDomain,
	})

	// Rutas

	// Middleware: cookie anónimo (GLOBAL — todas las rutas)
	// Skipper: no setear si el usuario ya tiene access_token
	anonSkipper := func(c *echo.Context) bool {
		if _, err := c.Cookie("__Secure-access_token"); err == nil {
			return true
		}
		if _, err := c.Cookie("access_token"); err == nil {
			return true
		}
		return false
	}
	anonCookieMW := ratelimit.AnonymousCookieMiddleware(anonSkipper, cfg.Server.Env == "production")
	e.Use(anonCookieMW)

	anonRateLimitMW := ratelimit.AnonymousRateLimitMiddleware(rateLimiter)

	// Middleware: rate limit autenticado (extrae user ID del PASETO)
	authRateLimitMW := ratelimit.AuthenticatedRateLimitMiddleware(rateLimiter,
		func(c *echo.Context) (string, bool) {
			cookie, err := c.Cookie("__Secure-access_token")
			if err != nil {
				cookie, err = c.Cookie("access_token")
			}
			if err != nil {
				return "", false
			}
			claims, err := authMod.TokenService.ValidateAccessToken(c.Request().Context(), cookie.Value)
			if err != nil {
				return "", false
			}
			return claims.UserID.String(), true
		},
	)

	// Auth routes: /v1/auth
	authGroup := e.Group("/v1/auth")

	// Register endpoint - usa RegisterHandler() que incluye soporte de idempotency
	authGroup.POST("/register", authMod.RegisterHandler().Handle, anonRateLimitMW)

	// Verify-email endpoint
	authGroup.POST("/verify-email", authMod.VerifyEmailHandler().Handle, anonRateLimitMW)

	// Login endpoint
	authGroup.POST("/login", authMod.LoginHandler.Handle, anonRateLimitMW)

	// Logout endpoints — auth middleware + authenticated rate limiting
	authGroup.POST("/logout", authMod.LogoutHandler.Handle, authMiddleware.Handle, authRateLimitMW)
	authGroup.POST("/logout/all", authMod.LogoutHandler.HandleAll, authMiddleware.Handle, authRateLimitMW)

	// OAuth endpoints — públicas, sin auth middleware (obviamente)
	authGroup.GET("/oauth/:provider", authMod.OAuthAuthorizeHandler.Handle, anonRateLimitMW)
	authGroup.GET("/oauth/:provider/callback", authMod.OAuthCallbackHandler.Handle)

	// Resend-verification endpoint — pública con rate limiting anónimo
	authGroup.POST("/resend-verification", authMod.ResendVerificationHandler.Handle, anonRateLimitMW)

	// Me endpoint — auth middleware + authenticated rate limiting
	authGroup.GET("/me", authMod.MeHandler.Handle, authMiddleware.Handle, authRateLimitMW)

	// Search routes: /v1/search (públicas con rate limit)
	searchGroup := e.Group("/v1/search", anonRateLimitMW, authMiddleware.Optional())
	searchGroup.POST("/flights", searchMod.SearchFlightsHandler.Handle)
	searchGroup.POST("/flight-details", searchMod.FlightDetailsHandler.Handle)
	searchGroup.POST("/hotels", searchMod.SearchHotelsHandler.Handle)
	searchGroup.POST("/hotel-details", searchMod.HotelDetailsHandler.Handle)
	// AI search — supports streaming via "stream": true in the request body
	searchGroup.POST("/ai", searchMod.AISearchHandler.Handle)

	// Execute saved search — requires auth (cookie), not optional
	if searchMod.ExecuteSavedSearchHandler != nil {
		searchGroup.POST("/execute_saved", searchMod.ExecuteSavedSearchHandler.Handle, authMiddleware.Handle, authRateLimitMW)
	}

	// Environment route: location + weather (public, no auth required)
	environmentGroup := e.Group("/v1")
	environmentGroup.GET("/environment", environmentMod.GetEnvironmentHandler.Handle)

	// User routes: /v1/user/profile/* (autenticado via cookie)
	userGroup := e.Group("/v1/user", authMiddleware.Handle, authRateLimitMW)
	userMod.RegisterRoutes(userGroup, authMiddleware.Handle)

	// Start conversation consumer (BACKGROUND - persiste conversaciones en PG via streams)
	if searchMod.ConvConsumer != nil {
		if err := searchMod.ConvConsumer.Start(appCtx); err != nil {
			slog.Warn("conversation consumer failed to start", "error", err)
			// No es fatal - el servidor puede iniciar sin el consumer
		}
	}

	// === App Structure ===
	app := &App{
		Echo:               e,
		RedisClient:        rdb,
		EventBus:           eventBus,
		Cfg:                cfg,
		DB:                 poolMgr,
		AuthModule:         authMod,
		UserModule:         userMod,
		NotificationModule: notifMod,
		SearchModule:       searchMod,
		EnvironmentModule:  environmentMod,
		appCancel:          appCancel,
		startTime:          time.Now(),
	}

	cancelOnError = false // app owns the cancel now — Shutdown will call it

	// Health checks (registered after app creation so methods can access modules)
	e.GET("/health", app.healthCheckHandler())
	e.GET("/ready", app.readyCheckHandler(rdb, poolMgr))

	slog.Info("App initialized", "modules", []string{"auth", "user", "notification", "search", "environment"})

	return app, nil
}

// healthCheckHandler returns basic liveness info (version, uptime, env).
func (app *App) healthCheckHandler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return c.JSON(http.StatusOK, struct {
			Status      string `json:"status"`
			Environment string `json:"environment"`
			Version     string `json:"version"`
			Uptime      string `json:"uptime"`
		}{
			Status:      "ok",
			Environment: app.Cfg.Server.Env,
			Version:     "0.1.0",
			Uptime:      time.Since(app.startTime).String(),
		})
	}
}

// readyCheckHandler verifies all infrastructure dependencies are healthy:
// Redis ping, all PostgreSQL pools, and event consumer goroutines.
func (app *App) readyCheckHandler(rdb *redis.Client, poolMgr *database.PoolManager) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
		defer cancel()

		checks := make(map[string]string)

		// Check Redis
		if err := rdb.Ping(ctx).Err(); err != nil {
			checks["redis"] = fmt.Sprintf("error: %v", err)
		} else {
			checks["redis"] = "ok"
		}

		// Check all DBs via PoolManager
		dbResults := poolMgr.HealthCheck(ctx)
		for dbType, err := range dbResults {
			if err != nil {
				checks[string(dbType)] = fmt.Sprintf("error: %v", err)
			} else {
				checks[string(dbType)] = "ok"
			}
		}

		// Check event consumers (notification + user + conversation)
		if app.NotificationModule != nil && app.NotificationModule.EventConsumer != nil {
			nc := app.NotificationModule.EventConsumer
			if nc.IsRunning() {
				checks[nc.Name()] = "ok"
			} else {
				checks[nc.Name()] = "error: consumer not running"
			}
		}

		if app.UserModule != nil && app.UserModule.EventConsumer != nil {
			uc := app.UserModule.EventConsumer
			if uc.IsRunning() {
				checks[uc.Name()] = "ok"
			} else {
				checks[uc.Name()] = "error: consumer not running"
			}
		}

		if app.SearchModule != nil && app.SearchModule.ConvConsumer != nil {
			cc := app.SearchModule.ConvConsumer
			if cc.IsRunning() {
				checks[cc.Name()] = "ok"
			} else {
				checks[cc.Name()] = "error: consumer not running"
			}
		}

		// Determine overall status
		var failed []string
		for component, status := range checks {
			if status != "ok" {
				failed = append(failed, component)
			}
		}

		statusCode := http.StatusOK
		overall := "ready"
		if len(failed) > 0 {
			statusCode = http.StatusServiceUnavailable
			overall = "degraded"
		}

		return c.JSON(statusCode, struct {
			Status   string            `json:"status"`
			Checks   map[string]string `json:"checks"`
			Failures []string          `json:"failures"`
		}{
			Status:   overall,
			Checks:   checks,
			Failures: failed,
		})
	}
}

// Start inicia el servidor con graceful shutdown
func (app *App) Start(ctx context.Context) error {
	sc := echo.StartConfig{
		Address:         ":" + app.Cfg.Server.Port,
		GracefulTimeout: 10 * time.Second,
	}

	ctxShutdown, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.Info("Server starting", "port", app.Cfg.Server.Port)
	if err := sc.Start(ctxShutdown, app.Echo); err != nil && err != http.ErrServerClosed {
		return err
	}
	slog.Info("Server stopped")
	return nil
}

// Shutdown hace cleanup de recursos
func (app *App) Shutdown(ctx context.Context) error {
	slog.Info("Shutting down app...")

	// Cancel background consumers (notification, user event handlers)
	if app.appCancel != nil {
		app.appCancel()
	}

	// Wait for pipeline workers to drain (graceful shutdown with timeout)
	if app.UserModule != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := app.UserModule.Shutdown(shutdownCtx); err != nil {
			slog.Warn("user module shutdown timed out", "error", err)
		}
	}

	// Wait for in-flight fire-and-forget goroutines (cache writes, history saves)
	if app.SearchModule != nil {
		app.SearchModule.Wait()
	}

	if app.RedisClient != nil {
		app.RedisClient.Close()
	}

	if app.DB != nil {
		app.DB.Close()
	}

	return nil
}
