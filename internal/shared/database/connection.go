package database

// =============================================================================
// Creación de pool de conexiones PostgreSQL
// Configura timeouts y statement timeouts por conexión
// =============================================================================

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewConnection crea un pool de conexiones con configuración personalizada.
// Incluye AfterConnect hook para configurar timeouts por defecto.
func NewConnection(ctx context.Context, url string, maxOpen, minConns, maxIdle int, maxLifetime, maxIdleTime time.Duration) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("error parseando URL: %w", err)
	}

	poolConfig.MaxConns = int32(maxOpen)
	poolConfig.MinConns = int32(minConns)
	poolConfig.MaxConnLifetime = maxLifetime
	poolConfig.MaxConnIdleTime = maxIdleTime

	// AfterConnect hook: se ejecuta cada vez que se crea una nueva conexión
	// Importante para establecer timeouts por defecto en cada conexión
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx,
			"SET statement_timeout = '30s'; SET idle_in_transaction_session_timeout = '60s'; SET application_name = 'proactrip-api';")
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("error creando pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping falló: %w", err)
	}

	return pool, nil
}
