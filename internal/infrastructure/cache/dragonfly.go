package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ProacTrip/Backend/config"
	cacheport "github.com/ProacTrip/Backend/internal/shared/domain/ports/cache"
	"github.com/redis/go-redis/v9"
)

// Aseguramos en tiempo de compilación que dragonflyAdapter implementa tu Cache
var _ cacheport.Cache = (*dragonflyAdapter)(nil)

type dragonflyAdapter struct {
	client *redis.Client
}

// NewDragonflyCache crea una instancia de DragonflyCache
func NewDragonflyCache(ctx context.Context, cfg *config.Config) (cacheport.Cache, error) {
	opts, err := redis.ParseURL(cfg.Cache.URL)
	if err != nil {
		return nil, fmt.Errorf("URL de caché inválida: %w", err)
	}

	opts.PoolSize = cfg.Cache.PoolSize
	opts.DialTimeout = 5 * time.Second

	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("error conectando a Dragonfly Cache: %w", err)
	}

	slog.Info("Conexión a Caché establecida", "db", "0")

	return &dragonflyAdapter{client: client}, nil
}

// Get recupera un valor del cache
func (d *dragonflyAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, cacheport.ErrInvalidKey
	}

	res, err := d.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, cacheport.ErrCacheMiss
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, cacheport.ErrCacheTimeout
	}
	if errors.Is(err, redis.ErrClosed) {
		return nil, cacheport.ErrCacheClosed
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", cacheport.ErrCacheInternal, err)
	}

	return []byte(res), nil
}

// Set almacena un valor en el cache con TTL opcional
func (d *dragonflyAdapter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if key == "" {
		return cacheport.ErrInvalidKey
	}
	if value == nil {
		return cacheport.ErrInvalidValue
	}
	if ttl < 0 {
		return cacheport.ErrInvalidTTL
	}

	err := d.client.Set(ctx, key, value, ttl).Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return cacheport.ErrCacheTimeout
	}
	if errors.Is(err, redis.ErrClosed) {
		return cacheport.ErrCacheClosed
	}
	if err != nil {
		return fmt.Errorf("%w: %v", cacheport.ErrCacheInternal, err)
	}

	return nil
}

// SetNX almacena un valor solo si la clave no existe
func (d *dragonflyAdapter) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if key == "" {
		return false, cacheport.ErrInvalidKey
	}
	if value == nil {
		return false, cacheport.ErrInvalidValue
	}
	if ttl < 0 {
		return false, cacheport.ErrInvalidTTL
	}

	cmd := d.client.SetArgs(ctx, key, value, redis.SetArgs{
		Mode: "NX",
		TTL:  ttl,
	})

	if err := cmd.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, cacheport.ErrCacheTimeout
		}
		if errors.Is(err, redis.ErrClosed) {
			return false, cacheport.ErrCacheClosed
		}
		return false, fmt.Errorf("%w: %v", cacheport.ErrCacheInternal, err)
	}

	return cmd.Val() == "OK", nil
}

// Delete elimina una o más claves del cache
func (d *dragonflyAdapter) Delete(ctx context.Context, keys ...string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	result, err := d.client.Del(ctx, keys...).Result()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, cacheport.ErrCacheTimeout
		}
		if errors.Is(err, redis.ErrClosed) {
			return 0, cacheport.ErrCacheClosed
		}
		return 0, fmt.Errorf("%w: %v", cacheport.ErrCacheInternal, err)
	}

	return result, nil
}

// Exists verifica si una o más claves existen
func (d *dragonflyAdapter) Exists(ctx context.Context, keys ...string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	result, err := d.client.Exists(ctx, keys...).Result()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, cacheport.ErrCacheTimeout
		}
		if errors.Is(err, redis.ErrClosed) {
			return 0, cacheport.ErrCacheClosed
		}
		return 0, fmt.Errorf("%w: %v", cacheport.ErrCacheInternal, err)
	}

	return result, nil
}

// Expire establece un TTL para una clave existente
func (d *dragonflyAdapter) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if key == "" {
		return cacheport.ErrInvalidKey
	}
	if ttl < 0 {
		return cacheport.ErrInvalidTTL
	}

	result, err := d.client.Expire(ctx, key, ttl).Result()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return cacheport.ErrCacheTimeout
		}
		if errors.Is(err, redis.ErrClosed) {
			return cacheport.ErrCacheClosed
		}
		return fmt.Errorf("%w: %v", cacheport.ErrCacheInternal, err)
	}

	if !result {
		return cacheport.ErrCacheMiss
	}

	return nil
}

// TTL retorna el tiempo restante de vida de una clave
func (d *dragonflyAdapter) TTL(ctx context.Context, key string) (time.Duration, error) {
	if key == "" {
		return 0, cacheport.ErrInvalidKey
	}

	ttl, err := d.client.TTL(ctx, key).Result()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, cacheport.ErrCacheTimeout
		}
		if errors.Is(err, redis.ErrClosed) {
			return 0, cacheport.ErrCacheClosed
		}
		return 0, fmt.Errorf("%w: %v", cacheport.ErrCacheInternal, err)
	}

	return ttl, nil
}

// Close cierra la conexión al servidor de cache
func (d *dragonflyAdapter) Close() error {
	slog.Info("Cerrando cliente de Cache")
	return d.client.Close()
}

// HealthCheck verifica la conectividad con el servidor
func (d *dragonflyAdapter) HealthCheck(ctx context.Context) error {
	return d.client.Ping(ctx).Err()
}
