// Controlador HTTP de Echo v5 para el endpoint en tiempo real de SSE.
package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
)

// Handler devuelve un controlador de Echo para GET /v1/realtime/events.
// Soporta tanto usuarios autenticados (via access_token cookie) como anónimos
// (via anon_token cookie). Para anónimos, el userID es el valor del anon_token.
func Handler(hub *Hub) echo.HandlerFunc {
	return func(c *echo.Context) error {
		// 1. Resolver userID: auth claims primero, luego anon_token cookie.
		// Si no hay ninguno, generar un UUID temporal para permitir la conexión.
		var userID string
		claims, err := sharedauth.GetAccessClaims(c)
		if err == nil && claims != nil {
			userID = claims.UserID.String()
		} else {
			userID = ratelimit.AnonIDFromContext(c)
		}
		if userID == "" {
			// No identifier at all — generate a temporary one.
			// The anon_token cookie will be set by the first search request.
			userID = uuid.New().String()
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

		// 3. Suscribirse al hub (userID must be a valid UUID).
		uid, parseErr := uuid.Parse(userID)
		if parseErr != nil {
			slog.Warn("sse: invalid user_id format",
				slog.String("user_id", userID),
				slog.String("error", parseErr.Error()))
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "invalid user identifier",
			})
		}
		slog.Info("sse: subscribed", "user_id", uid.String())
		ch := hub.Subscribe(uid)
		defer hub.Unsubscribe(uid, ch)

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
					slog.String("user_id", userID),
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
