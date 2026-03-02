package cache

import (
	"context"
	"time"
)

// Cache define la interfaz para operaciones de cache
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	Delete(ctx context.Context, keys ...string) (int64, error)
	Exists(ctx context.Context, keys ...string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	TTL(ctx context.Context, key string) (time.Duration, error)
	Close() error
	HealthCheck(ctx context.Context) error
}

// CacheError representa errores específicos del cache
type CacheError string

func (e CacheError) Error() string { return string(e) }

// Errores comunes del cache
const (
	ErrCacheMiss     CacheError = "cache: key not found"
	ErrCacheClosed   CacheError = "cache: connection closed"
	ErrInvalidTTL    CacheError = "cache: invalid TTL"
	ErrInvalidKey    CacheError = "cache: invalid key"
	ErrInvalidValue  CacheError = "cache: invalid value"
	ErrCacheTimeout  CacheError = "cache: operation timeout"
	ErrCacheInternal CacheError = "cache: generic internal errors"
)
