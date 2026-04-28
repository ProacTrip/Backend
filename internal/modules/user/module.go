// Módulo de usuario.
// Maneja perfiles de usuario y sus preferencias.
// Usa PostgreSQL para persistencia.
package user

import (
	"log/slog"

	"github.com/ProacTrip/Backend/internal/modules/user/adapters/postgres"
	"github.com/ProacTrip/Backend/internal/modules/user/consumer"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	"github.com/ProacTrip/Backend/internal/modules/user/features/upsert_profile"
	"github.com/ProacTrip/Backend/internal/shared/eventbus"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Module contiene las dependencias del módulo User
type Module struct {
	// Repository (interfaz del dominio)
	Repository domain.UserRepository

	// Use Cases
	UpsertProfileUseCase *upsert_profile.UseCase

	// Event Consumer
	EventConsumer *consumer.UserEventConsumer
}

// Config configuración del módulo
type Config struct {
	PostgresPool *pgxpool.Pool
	RedisClient  *redis.Client
	EventBus     *eventbus.EventBus
}

// NewModule crea e inicializa el módulo User
func NewModule(cfg Config) (*Module, error) {
	m := &Module{}

	// 1. Inicializar Repository (PostgreSQL adapter)
	m.Repository = postgres.NewUserRepository(cfg.PostgresPool)

	// 2. Inicializar Use Case
	m.UpsertProfileUseCase = upsert_profile.NewUseCase(m.Repository)

	// 3. Inicializar Event Consumer (Dragonfly Streams)
	m.EventConsumer = consumer.NewUserEventConsumer(cfg.RedisClient, m.Repository)

	slog.Info("User module initialized",
		"features", []string{"upsert_profile"},
		"consumer", "user-event-consumer",
	)

	return m, nil
}
