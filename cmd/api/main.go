package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ProacTrip/Backend/config"
	"github.com/ProacTrip/Backend/internal/infrastructure/cache"
	"github.com/ProacTrip/Backend/internal/infrastructure/database"
	"github.com/ProacTrip/Backend/internal/infrastructure/eventbus"
	"github.com/ProacTrip/Backend/internal/infrastructure/health"
	"github.com/ProacTrip/Backend/internal/shared/api"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Error crítico en la aplicación", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// === 1. Configuración ===
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// === 2. Contexto con timeout para todo el arranque ===
	startupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// === 3. Inicializar infraestructura (con timeout) ===
	pm, err := database.NewPoolManager(cfg)
	if err != nil {
		return err
	}
	defer pm.Close()

	cacheInst, err := cache.New(startupCtx, cfg)
	if err != nil {
		return err
	}
	defer cacheInst.Close()

	eb, err := eventbus.New(startupCtx, cfg)
	if err != nil {
		return err
	}
	defer eb.Close()

	healthChecker := health.NewChecker(pm, cacheInst, eb)

	// === 4. Echo + rutas + middlewares + versionado ===
	e := echo.New()
	setupMiddlewares(e, cfg)
	setupRoutes(e, healthChecker, cfg)

	// === 5. Servidor ===
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      e,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Iniciar servidor
	go func() {
		slog.Info("🚀 Servidor iniciado",
			"app", cfg.App.Name,
			"version", cfg.App.Version,
			"port", cfg.Server.Port,
			"env", cfg.App.Environment,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Servidor falló", "error", err)
		}
	}()

	// === 6. Graceful Shutdown ===
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop() // Importante: restaura el comportamiento por defecto de las señales al salir

	// Bloquear el hilo principal hasta que llegue la señal de apagado
	<-ctx.Done()

	slog.Info("🔻 Señal de apagado recibida. Iniciando shutdown graceful...")

	// Crear el contexto con el timeout específico para esperar a las peticiones HTTP activas
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Error en shutdown del servidor HTTP", "error", err)
	}

	slog.Info("✅ Servidor apagado correctamente")
	return nil
}

// === Middlewares y Rutas (igual que antes) ===
func setupMiddlewares(e *echo.Echo, cfg *config.Config) {
	origins := []string{cfg.App.FrontendURL}
	if cfg.App.Environment == "development" {
		origins = []string{"*"}
	}

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:      true,
		LogStatus:   true,
		LogMethod:   true,
		LogLatency:  true,
		LogRemoteIP: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			if v.URI == "/favicon.ico" || v.URI == "/health" {
				return nil
			}
			slog.Info("REQUEST",
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
				"latency", v.Latency,
				"ip", v.RemoteIP,
			)
			return nil
		},
	}))
	e.Use(middleware.Secure())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: origins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
	}))
	e.Use(middleware.Gzip())
}

func setupRoutes(e *echo.Echo, hc *health.Checker, cfg *config.Config) {
	e.GET("/health", func(c *echo.Context) error {
		timeoutCtx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
		defer cancel()

		resp := hc.Check(timeoutCtx)

		status := http.StatusOK
		if resp.Status != "healthy" {
			status = http.StatusServiceUnavailable
		}
		return c.JSON(status, resp)
	})

	// === Versionado de API ===
	apiPrefix := api.Prefix(cfg.App.Version) // → "/v1"
	v1 := e.Group(apiPrefix)

	v1.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "API "+cfg.App.Name+" v"+cfg.App.Version+" funcionando correctamente")
	})
}
