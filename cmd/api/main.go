package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ProacTrip/Backend/internal/bootstrap"
	"github.com/ProacTrip/Backend/internal/config"
	"github.com/joho/godotenv"
)

var (
	version   = "dev"
	buildTime = "now"
)

func main() {
	env := flag.String("env", "", "Environment: development, staging, production")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		slog.Info("ProacTrip Backend", "version", version, "build_time", buildTime)
		os.Exit(0)
	}

	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: .env file not loaded (running without Docker?): %v\n", err)
	}

	// Environment: CLI flag > env var > default
	envValue := *env
	if envValue == "" {
		envValue = os.Getenv("APP_ENV")
	}
	if envValue == "" {
		envValue = "development"
	}

	// Setup logger — set as default so all packages use structured slog
	logger := setupLogger(envValue)
	slog.SetDefault(logger)

	// Load config
	cfg := config.Load()
	if err := config.ValidateSecureConfig(cfg); err != nil {
		logger.Error("invalid configuration", "error", err, "env", envValue)
		os.Exit(1)
	}

	app, err := bootstrap.NewApp(cfg, logger)
	if err != nil {
		logger.Error("error initializing application", "error", err)
		os.Exit(1)
	}

	// Graceful shutdown
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.Shutdown(shutdownCtx); err != nil {
			logger.Error("error during shutdown", "error", err)
		}
	}()

	// Start server
	if err := app.Start(context.Background()); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(env string) *slog.Logger {
	var handler slog.Handler

	switch env {
	case "staging", "production":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					a.Value = slog.StringValue(time.Now().Format("15:04:05.000"))
				}
				return a
			},
		})
	}

	return slog.New(handler)
}
