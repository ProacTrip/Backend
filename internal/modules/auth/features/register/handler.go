package register

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/auth/domain"
	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"
)

// Handler HTTP para registro de usuarios.
// Valida input, maneja idempotencia via Dragonfly. No emite cookies.

// IdempotencyConfig configuración para manejo de idempotencia
type IdempotencyConfig struct {
	RedisClient *redis.Client
	TTL         time.Duration // Por defecto: 24h
}

type Handler struct {
	usecase           *UseCase
	idempotencyConfig *IdempotencyConfig
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase, idempotencyConfig: nil}
}

// NewHandlerWithIdempotency crea handler con soporte de idempotencia
func NewHandlerWithIdempotency(usecase *UseCase, rdb *redis.Client) *Handler {
	return &Handler{
		usecase: usecase,
		idempotencyConfig: &IdempotencyConfig{
			RedisClient: rdb,
			TTL:         24 * time.Hour,
		},
	}
}

// Handle procesa las requests de registro según AUTH_API.md
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	// Validar que Idempotency-Key esté presente — requerido según spec.
	idempotencyKey := c.Request().Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		return httperr.MapError(c, fmt.Errorf("%w: el header Idempotency-Key es requerido", domain.ErrInvalidInput))
	}

	if h.idempotencyConfig != nil {
		cachedResp, err := h.getCachedResponse(c.Request().Context(), idempotencyKey)
		if err == nil && cachedResp != nil {
			return c.JSON(http.StatusCreated, cachedResp)
		}
	}

	var cmd Command

	if err := c.Bind(&cmd); err != nil {
		return httperr.MapError(c, err)
	}

	if err := cmd.Validate(); err != nil {
		return httperr.MapError(c, err)
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd)
	if err != nil {
		return httperr.MapError(c, err)
	}

	// Sin cookies — el registro no crea sesión.
	if err := c.JSON(http.StatusCreated, resp); err != nil {
		return err
	}

	if h.idempotencyConfig != nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request().Context()), 5*time.Second)
		go func() {
			defer cancel()
			h.cacheResponse(ctx, idempotencyKey, resp)
		}()
	}

	return nil
}

// getCachedResponse recupera la respuesta cacheada desde Dragonfly
func (h *Handler) getCachedResponse(ctx context.Context, key string) (map[string]any, error) {
	if h.idempotencyConfig == nil || h.idempotencyConfig.RedisClient == nil {
		return nil, errors.New("idempotency not configured")
	}

	val, err := h.idempotencyConfig.RedisClient.Get(ctx, "{idempotency}:register:"+key).Result()
	if err == redis.Nil {
		return nil, errors.New("not cached")
	}
	if err != nil {
		return nil, err
	}

	// Parsear JSON a map
	var result map[string]any
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, err
	}

	return result, nil
}

// cacheResponse almacena la respuesta completa en Dragonfly (fire and forget)
func (h *Handler) cacheResponse(ctx context.Context, key string, response *Response) {
	if h.idempotencyConfig == nil || h.idempotencyConfig.RedisClient == nil {
		return
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return
	}

	err = h.idempotencyConfig.RedisClient.Set(ctx,
		"{idempotency}:register:"+key,
		string(jsonBytes),
		h.idempotencyConfig.TTL,
	).Err()

	if err != nil {
		// Log pero no fallar — la idempotencia es best-effort
	}
}
