// Módulo de notificaciones.
// Maneja el envío de emails y gestión de estados de entrega.
// Usa PostgreSQL para persistencia y Resend para envío de emails.
package notification

import (
	"context"
	"log/slog"

	"github.com/ProacTrip/Backend/internal/config"
	"github.com/ProacTrip/Backend/internal/modules/notification/adapters/email"
	"github.com/ProacTrip/Backend/internal/modules/notification/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/notification/consumer"
	"github.com/ProacTrip/Backend/internal/modules/notification/features/send_verification_email"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Module contiene las dependencias del módulo Notification.
type Module struct {
	// Repository
	Repository *postgres.NotificationRepository

	// Email Sender
	EmailSender *email.ResendService

	// Use Cases
	SendVerificationEmailUseCase *send_verification_email.UseCase

	// Event Consumer
	EventConsumer *consumer.NotificationConsumer

	// Control de apagado del consumer
	cancel context.CancelFunc
}

// Config contiene la configuración necesaria para inicializar el módulo de notificaciones.
type Config struct {
	PostgresPool   *pgxpool.Pool
	RedisClient    *redis.Client
	ResendAPIKey   string
	FrontendConfig config.FrontendConfig
	RateLimiter    *ratelimit.RateLimiter
}

// NewModule crea e inicializa el módulo Notification
func NewModule(cfg Config) (*Module, error) {
	m := &Module{}

	// Control de apagado del consumer
	_, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// 1. Inicializar Repository (PostgreSQL adapter)
	m.Repository = postgres.NewNotificationRepository(cfg.PostgresPool)

	// 2. Inicializar Email Sender (Resend adapter)
	m.EmailSender = email.NewResendService(email.ResendConfig{
		APIKey:      cfg.ResendAPIKey,
		RateLimiter: cfg.RateLimiter,
	})

	// 3. Inicializar Use Case con dependencias
	m.SendVerificationEmailUseCase = send_verification_email.NewUseCase(
		send_verification_email.Deps{
			Repo:           m.Repository,
			Sender:         m.EmailSender,
			FrontendConfig: cfg.FrontendConfig,
		},
	)

	// 4. Inicializar Event Consumer (Dragonfly Streams)
	m.EventConsumer = consumer.NewNotificationConsumer(cfg.RedisClient, m.SendVerificationEmailUseCase)

	slog.Info("Notification module initialized",
		"features", []string{"send_verification_email"},
		"consumer", "notification-consumer",
	)

	return m, nil
}

// Shutdown detiene el consumer de eventos y libera recursos del módulo.
func (m *Module) Shutdown(ctx context.Context) error {
	slog.Info("notification module: iniciando apagado del consumer")
	m.cancel()
	return nil
}
