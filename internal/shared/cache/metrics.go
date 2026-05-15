// Decorador de métricas para el puerto Cache.
// Envuelve una implementación de Cache y expone contadores
// hit/miss/error/set via expvar (/debug/vars).
package cache

import (
	"context"
	"errors"
	"expvar"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Puerto Cache — interfaz local para operaciones de caché
// =============================================================================

// Cache define el puerto de caché usado por los use cases.
// Implementado por Dragonfly y por MetricsDecorator.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// Compile-time check: Dragonfly implements Cache.
var _ Cache = (*Dragonfly)(nil)

// =============================================================================
// Métricas de caché — contadores expuestos vía expvar
// =============================================================================

var (
	cacheHits      = expvar.NewInt("cache_hits")
	cacheMisses    = expvar.NewInt("cache_misses")
	cacheErrors    = expvar.NewInt("cache_errors")
	cacheSets      = expvar.NewInt("cache_sets")
	cacheSetErrors = expvar.NewInt("cache_set_errors")
)

// MetricsDecorator envuelve una implementación de Cache y registra métricas
// de hit/miss/error para Get y Set.
type MetricsDecorator struct {
	inner Cache
}

// NewMetricsDecorator crea un decorador que envuelve inner y cuenta hits,
// misses y errores vía expvar.
func NewMetricsDecorator(inner Cache) *MetricsDecorator {
	return &MetricsDecorator{inner: inner}
}

// Get obtiene un valor de la caché subyacente y actualiza los contadores
// de hit, miss o error según corresponda.
// La caché subyacente (Dragonfly) retorna "", nil en cache miss:
//   - Hit:  val != "" y err == nil
//   - Miss: val == "" y err == nil (contracto Dragonfly) o err == redis.Nil
//   - Error: cualquier otro error
func (d *MetricsDecorator) Get(ctx context.Context, key string) (string, error) {
	val, err := d.inner.Get(ctx, key)
	if err == nil && val != "" {
		cacheHits.Add(1)
		return val, nil
	}
	if err == nil || errors.Is(err, redis.Nil) {
		cacheMisses.Add(1)
		return "", nil
	}
	cacheErrors.Add(1)
	return "", err
}

// Set almacena un valor en la caché subyacente y actualiza los contadores
// de sets exitosos o con error.
func (d *MetricsDecorator) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	err := d.inner.Set(ctx, key, value, ttl)
	if err != nil {
		cacheSetErrors.Add(1)
		return err
	}
	cacheSets.Add(1)
	return nil
}
