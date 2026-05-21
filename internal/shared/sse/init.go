// Hub singleton para el paquete de tiempo real SSE.
package sse

var defaultHub *Hub

// Init crea y almacena el Hub singleton.
// Debe llamarse durante el arranque (bootstrap) antes del registro de rutas.
func Init() {
	defaultHub = NewHub()
}

// GetHub devuelve el Hub singleton.
// Lanza un pánico si no se llamó primero a Init().
func GetHub() *Hub {
	if defaultHub == nil {
		panic("sse: Init() must be called before GetHub()")
	}
	return defaultHub
}
