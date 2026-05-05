package register

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	httperr "github.com/ProacTrip/Backend/internal/shared/http"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"
)

// Handler HTTP para registro de usuarios.
// Valida input, maneja idempotencia via Dragonfly y setea cookies en producción.

// IdempotencyConfig configuración para manejo de idempotencia
type IdempotencyConfig struct {
	RedisClient *redis.Client
	TTL         time.Duration // Default: 24h
}

type Handler struct {
	usecase           *UseCase
	idempotencyConfig *IdempotencyConfig
	isProduction      bool
}

func NewHandler(usecase *UseCase) *Handler {
	return &Handler{usecase: usecase, idempotencyConfig: nil, isProduction: false}
}

// NewHandlerWithIdempotency crea handler con soporte de idempotencia
func NewHandlerWithIdempotency(usecase *UseCase, rdb *redis.Client, isProduction bool) *Handler {
	return &Handler{
		usecase: usecase,
		idempotencyConfig: &IdempotencyConfig{
			RedisClient: rdb,
			TTL:         24 * time.Hour,
		},
		isProduction: isProduction,
	}
}

// Handle procesa las requests de registro según AUTH_API.md
func (h *Handler) Handle(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store, private")

	envIP := c.RealIP()

	idempotencyKey := c.Request().Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = uuid.Must(uuid.NewV7()).String()
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

	if cmd.Email == "" || cmd.Password == "" {
		return httperr.MapError(c, echo.NewHTTPError(http.StatusBadRequest, "email and password are required"))
	}

	resp, err := h.usecase.Execute(c.Request().Context(), cmd, envIP)
	if err != nil {
		return httperr.MapError(c, err)
	}

	if resp.AccessToken != "" && resp.RefreshToken != "" {
		if h.isProduction {
			httperr.SetAuthCookiesFromTokens(c, resp.AccessToken, resp.RefreshToken)
		} else {
			httperr.SetAuthCookiesDev(c, resp.AccessToken, resp.RefreshToken)
		}
	}

	if err := c.JSON(http.StatusCreated, resp); err != nil {
		return err
	}

	if h.idempotencyConfig != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		go func() {
			defer cancel()
			h.cacheResponse(ctx, idempotencyKey, resp)
		}()
	}

	return nil
}

// getCachedResponse retrieves cached response from Dragonfly
func (h *Handler) getCachedResponse(ctx context.Context, key string) (map[string]interface{}, error) {
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

	// Parse JSON string back to map
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, err
	}

	return result, nil
}

// cacheResponse stores full response in Dragonfly (fire and forget)
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
		// Log but don't fail - idempotency is best-effort
	}
}
