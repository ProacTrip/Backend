package database

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolManager gestiona las 8 bases de datos lógicas en una sola instancia Postgres
type PoolManager struct {
	pools map[DBType]*pgxpool.Pool
}

// NewPoolManager crea y configura todos los pools (usado solo en el monolith)
func NewPoolManager(cfg *config.Config) (*PoolManager, error) {
	manager := &PoolManager{
		pools: make(map[DBType]*pgxpool.Pool),
	}

	databases := map[DBType]struct {
		URL             string
		MaxOpenConns    int
		MaxIdleConns    int
		MaxConnLifetime time.Duration
		MaxConnIdleTime time.Duration
	}{
		DBAuth:         {cfg.Database.Auth, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.MaxConnLifetime, cfg.Database.MaxConnIdleTime},
		DBReference:    {cfg.Database.Reference, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.MaxConnLifetime, cfg.Database.MaxConnIdleTime},
		DBUser:         {cfg.Database.User, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.MaxConnLifetime, cfg.Database.MaxConnIdleTime},
		DBBooking:      {cfg.Database.Booking, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.MaxConnLifetime, cfg.Database.MaxConnIdleTime},
		DBPayment:      {cfg.Database.Payment, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.MaxConnLifetime, cfg.Database.MaxConnIdleTime},
		DBSearch:       {cfg.Database.Search, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.MaxConnLifetime, cfg.Database.MaxConnIdleTime},
		DBNotification: {cfg.Database.Notification, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.MaxConnLifetime, cfg.Database.MaxConnIdleTime},
		DBAudit:        {cfg.Database.Audit, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.MaxConnLifetime, cfg.Database.MaxConnIdleTime},
	}

	for dbType, dbCfg := range databases {
		pool, err := NewConnection(context.Background(), dbCfg.URL, dbCfg.MaxOpenConns, dbCfg.MaxIdleConns, dbCfg.MaxConnLifetime, dbCfg.MaxConnIdleTime)
		if err != nil {
			manager.Close()
			return nil, fmt.Errorf("error creando pool para %s: %w", dbType, err)
		}
		manager.pools[dbType] = pool
		slog.Info("Pool de conexiones creado", "database", dbType)
	}

	return manager, nil
}

// GetPool retorna el pool de una base de datos específica
func (pm *PoolManager) GetPool(dbType DBType) (*pgxpool.Pool, error) {
	pool, ok := pm.pools[dbType]
	if !ok {
		return nil, fmt.Errorf("pool no encontrado: %s", dbType)
	}
	return pool, nil
}

// Close cierra todos los pools
func (pm *PoolManager) Close() {
	for dbType, pool := range pm.pools {
		if pool != nil {
			pool.Close()
			slog.Info("Pool cerrado", "database", dbType)
		}
	}
}

// HealthCheck realiza pings concurrentes a todas las DBs
func (pm *PoolManager) HealthCheck(ctx context.Context) map[DBType]error {
	results := make(map[DBType]error)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for dbType, pool := range pm.pools {
		wg.Add(1)
		go func(dt DBType, p *pgxpool.Pool) {
			defer wg.Done()
			var err error
			if p == nil {
				err = fmt.Errorf("pool no inicializado")
			} else {
				pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				err = p.Ping(pingCtx)
			}

			if err != nil {
				slog.Warn("HealthCheck falló", "database", dt, "error", err)
			} else {
				slog.Debug("HealthCheck ok", "database", dt)
			}

			mu.Lock()
			results[dt] = err
			mu.Unlock()
		}(dbType, pool)
	}
	wg.Wait()
	return results
}
