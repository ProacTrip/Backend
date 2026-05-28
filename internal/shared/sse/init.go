// Hub singleton para el paquete de tiempo real SSE.
package sse

import (
	"context"

	"github.com/redis/go-redis/v9"
)

var defaultHub *Hub

// Init crea y almacena el Hub singleton y arranca el bridge cross-instance.
// Debe llamarse durante el arranque (bootstrap) antes del registro de rutas.
// rdb puede ser nil: el bridge se convierte en no-op (solo entrega local).
func Init(ctx context.Context, rdb *redis.Client) {
	defaultHub = NewHub()
	NewBridge(defaultHub, rdb).Start(ctx)
}

// GetHub devuelve el Hub singleton.
// Lanza un pánico si no se llamó primero a Init().
func GetHub() *Hub {
	if defaultHub == nil {
		panic("sse: Init() must be called before GetHub()")
	}
	return defaultHub
}
