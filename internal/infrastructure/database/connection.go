package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewConnection crea un solo pool (ideal cuando extraigas un microservicio)
func NewConnection(ctx context.Context, url string, maxOpen, maxIdle int, maxLifetime, maxIdleTime time.Duration) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("error parseando URL: %w", err)
	}

	poolConfig.MaxConns = int32(maxOpen)
	poolConfig.MinConns = int32(maxIdle)
	poolConfig.MaxConnLifetime = maxLifetime
	poolConfig.MaxConnIdleTime = maxIdleTime

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
