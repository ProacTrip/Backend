package eventbus

import (
	"context"

	"github.com/ProacTrip/Backend/config"
	eventport "github.com/ProacTrip/Backend/internal/shared/domain/ports/eventbus"
)

// New crea una implementación de EventBus (Dragonfly por defecto)
func New(ctx context.Context, cfg *config.Config) (eventport.Bus, error) {
	return NewDragonflyBus(ctx, cfg.EventBus.URL, cfg.EventBus.PoolSize, cfg.EventBus.MaxLen)
}
