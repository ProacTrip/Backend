package cache

import (
	"context"

	"github.com/ProacTrip/Backend/config"
	cacheport "github.com/ProacTrip/Backend/internal/shared/domain/ports/cache"
)

// New crea una implementación de cache (Dragonfly por defecto)
func New(ctx context.Context, cfg *config.Config) (cacheport.Cache, error) {
	return NewDragonflyCache(ctx, cfg)
}
