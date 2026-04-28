package database

// =============================================================================
// Gestor de múltiples pools de bases de datos con lazy init
// Implementa circuit breaker para tolerate falhas transient
// =============================================================================

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolStats expone las estadísticas del pool en un formato serializable
type PoolStats struct {
	TotalConns           int32
	IdleConns            int32
	AcquiredConns        int32
	ConstructingConns    int32
	EmptyAcquireCount    int64
	CanceledAcquireCount int64
}

// circuitState mantiene el estado del circuit breaker para cada DB
type circuitState struct {
	open          atomic.Bool
	openUntilNano atomic.Int64
	backoffSec    atomic.Int64
}

func (c *circuitState) isOpen() bool {
	if !c.open.Load() {
		return false
	}
	if time.Now().UnixNano() > c.openUntilNano.Load() {
		c.open.Store(false)
		return false
	}
	return true
}

func (c *circuitState) openCircuit(duration time.Duration) {
	c.open.Store(true)
	c.openUntilNano.Store(time.Now().Add(duration).UnixNano())
}

func (c *circuitState) closeCircuit() {
	c.open.Store(false)
	c.backoffSec.Store(30)
}

func (c *circuitState) nextBackoff() time.Duration {
	current := c.backoffSec.Load()
	next := current * 2
	if next > 120 {
		next = 120
	}
	c.backoffSec.Store(next)
	return time.Duration(next) * time.Second
}

func (c *circuitState) initialBackoff() time.Duration {
	c.backoffSec.Store(30)
	return 30 * time.Second
}

// PoolConfig contiene la configuración para todas las bases de datos
type PoolConfig struct {
	Auth            string
	User            string
	Booking         string
	Payment         string
	Search          string
	Notification    string
	Audit           string
	MaxOpenConns    int
	MaxIdleConns    int
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// PoolManager gestiona múltiples bases de datos con lazy initialization
type PoolManager struct {
	mu              sync.RWMutex
	configs         map[DBType]DBConfig
	pools           map[DBType]*pgxpool.Pool
	circuitBreakers map[DBType]*circuitState
}

// NewPoolManager crea el manager pero NO inicializa las conexiones (lazy)
// cfg debe contener las URLs y configuración de todas las bases de datos
func NewPoolManager(cfg PoolConfig) *PoolManager {
	manager := &PoolManager{
		configs:         make(map[DBType]DBConfig),
		pools:           make(map[DBType]*pgxpool.Pool),
		circuitBreakers: make(map[DBType]*circuitState),
	}

	databases := map[DBType]string{
		DBAuth:         cfg.Auth,
		DBUser:         cfg.User,
		DBBooking:      cfg.Booking,
		DBPayment:      cfg.Payment,
		DBSearch:       cfg.Search,
		DBNotification: cfg.Notification,
		DBAudit:        cfg.Audit,
	}

	for dbType, url := range databases {
		if url != "" { // Solo registrar si hay URL configurada
			manager.configs[dbType] = DBConfig{
				URL:             url,
				MaxOpenConns:    cfg.MaxOpenConns,
				MaxIdleConns:    cfg.MaxIdleConns,
				MaxConnLifetime: cfg.MaxConnLifetime,
				MaxConnIdleTime: cfg.MaxConnIdleTime,
			}
			manager.circuitBreakers[dbType] = &circuitState{}
		}
	}

	return manager
}

// GetPool retorna el pool de una DB específica, iniciándolo si es necesario
func (pm *PoolManager) GetPool(dbType DBType) (*pgxpool.Pool, error) {
	pm.mu.RLock()
	pool, ok := pm.pools[dbType]
	pm.mu.RUnlock()

	if ok {
		return pool, nil
	}

	cb := pm.circuitBreakers[dbType]
	if cb != nil && cb.isOpen() {
		return nil, fmt.Errorf("circuit breaker open for %s, retry after cooldown", dbType)
	}

	// Crear pool on-demand
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check
	if pool, ok = pm.pools[dbType]; ok {
		return pool, nil
	}

	cfg, exists := pm.configs[dbType]
	if !exists {
		return nil, fmt.Errorf("configuración no encontrada para la db: %s", dbType)
	}

	minConns := max(cfg.MaxIdleConns/4, 2)

	var newPool *pgxpool.Pool
	var err error
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	for attempt, backoff := range append(backoffs, 0) {
		newPool, err = NewConnection(context.Background(), cfg.URL, cfg.MaxOpenConns, minConns, cfg.MaxIdleConns, cfg.MaxConnLifetime, cfg.MaxConnIdleTime)
		if err == nil {
			break
		}
		if attempt < len(backoffs) {
			slog.Warn("Transient pool creation failure, retrying", "database", dbType, "attempt", attempt+1, "error", err)
			time.Sleep(backoff)
		}
	}

	if err != nil {
		if cb != nil {
			if cb.backoffSec.Load() == 0 {
				cb.openCircuit(cb.initialBackoff())
			} else {
				cb.openCircuit(cb.nextBackoff())
			}
			slog.Warn("Circuit breaker opened for database pool", "database", dbType, "cooldown", cb.openUntilNano.Load())
		}
		return nil, fmt.Errorf("pool creation failed after retries for %s: %w", dbType, err)
	}

	if cb != nil {
		cb.closeCircuit()
	}

	pm.pools[dbType] = newPool
	slog.Info("Pool de conexiones inicializado lazy", "database", dbType)
	return newPool, nil
}

// Close cierra todos los pools activos
func (pm *PoolManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for dbType, pool := range pm.pools {
		if pool != nil {
			pool.Close()
			slog.Info("Pool cerrado", "database", dbType)
		}
	}
	pm.pools = make(map[DBType]*pgxpool.Pool)
}

// PoolStats retorna las estadísticas de todos los pools inicializados
func (pm *PoolManager) PoolStats() map[DBType]PoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[DBType]PoolStats, len(pm.pools))
	for dbType, pool := range pm.pools {
		if pool == nil {
			continue
		}
		stat := pool.Stat()
		result[dbType] = PoolStats{
			TotalConns:        stat.TotalConns(),
			IdleConns:         stat.IdleConns(),
			AcquiredConns:     stat.AcquiredConns(),
			ConstructingConns: stat.ConstructingConns(),
			EmptyAcquireCount: stat.EmptyAcquireCount(),
		}
	}
	return result
}

// HealthCheck realiza pings concurrentes a todas las DBs inicializadas
func (pm *PoolManager) HealthCheck(ctx context.Context) map[DBType]error {
	pm.mu.RLock()
	pools := make(map[DBType]*pgxpool.Pool, len(pm.pools))
	for k, v := range pm.pools {
		pools[k] = v
	}
	pm.mu.RUnlock()

	results := make(map[DBType]error)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for dbType, pool := range pools {
		wg.Go(func() {
			var err error
			if pool == nil {
				err = fmt.Errorf("pool no inicializado")
			} else {
				pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				err = pool.Ping(pingCtx)
			}

			if err != nil {
				slog.Warn("HealthCheck falló", "database", dbType, "error", err)
			}

			mu.Lock()
			results[dbType] = err
			mu.Unlock()
		})
	}
	wg.Wait()
	return results
}
