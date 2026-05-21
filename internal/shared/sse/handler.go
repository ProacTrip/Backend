// Controlador HTTP de Echo v5 para el endpoint en tiempo real de SSE.
package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

// Handler devuelve un controlador de Echo para GET /v1/realtime/events.
// Extrae los claims del usuario del middleware de autenticación, se suscribe al
// hub y transmite los eventos como SSE hasta que el cliente se desconecte.
func Handler(hub *Hub) echo.HandlerFunc {
	return func(c *echo.Context) error {
		// 1. Extraer los claims del usuario del contexto (establecidos por el middleware de autenticación).
		claims, err := sharedauth.GetAccessClaims(c)
		if err != nil {
			slog.Warn("sse: missing user_claims in context",
				slog.String("path", c.Request().URL.Path))
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "authentication required",
			})
		}

		// 2. Establecer las cabeceras de SSE.
		w := c.Response()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Asegurar que el escritor de respuestas admita flushing (requerido para SSE).
		flusher, ok := w.(http.Flusher)
		if !ok {
			return fmt.Errorf("sse: ResponseWriter does not implement http.Flusher")
		}

		// 3. Suscribirse al hub.
		ch := hub.Subscribe(claims.UserID)
		defer hub.Unsubscribe(claims.UserID, ch)

		// 4. Transmitir eventos en streaming hasta que el cliente se desconecte.
		ctx := c.Request().Context()
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					// Canal cerrado por Unsubscribe — salir normalmente.
					return nil
				}
				data, err := json.Marshal(event.Data)
				if err != nil {
					slog.Warn("sse: marshal event data failed",
						slog.String("user_id", claims.UserID.String()),
						slog.String("event_type", event.Type),
						slog.String("error", err.Error()),
					)
					continue
				}
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
				flusher.Flush()

			case <-ctx.Done():
				return nil
			}
		}
	}
}
