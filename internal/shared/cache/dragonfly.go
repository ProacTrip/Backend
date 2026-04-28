package cache

// =============================================================================
// Cliente de Dragonfly/Redis para cache y event bus
// Operaciones: strings, hashes, sets, counters con hashtags
// =============================================================================

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Dragonfly Client - Mejores prácticas v1.38
// =============================================================================

// Config contiene la configuración para la conexión a Dragonfly
type Config struct {
	URL          string
	Password     string
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultConfig crea una configuración con valores por defecto optimizados para Dragonfly
func DefaultConfig(addr, password string) *Config {
	return &Config{
		URL:          addr,
		Password:     password,
		PoolSize:     200, // Alto throughput para multi-threading de Dragonfly
		MinIdleConns: 20,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
}

// Dragonfly es el cliente principal de cache/event bus
type Dragonfly struct {
	client *redis.Client
}

// NewDragonfly crea un nuevo cliente de Dragonfly
func NewDragonfly(cfg *Config) (*Dragonfly, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.URL,
		Password:     cfg.Password,
		DB:           0,
		Protocol:     2, // REQUIRED para DIALECT 2 vector search
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("DragonflyDB connection failed: %w", err)
	}

	return &Dragonfly{client: client}, nil
}

// Client retorna el cliente Redis subyacente
func (d *Dragonfly) Client() *redis.Client {
	return d.client
}

// =============================================================================
// Cache Operations
// =============================================================================

// Get obtiene un valor de la cache
func (d *Dragonfly) Get(ctx context.Context, key string) (string, error) {
	val, err := d.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// Set establece un valor con TTL
func (d *Dragonfly) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return d.client.Set(ctx, key, value, ttl).Err()
}

// SetNX establece un valor solo si no existe (para distributed locks)
func (d *Dragonfly) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	return d.client.SetNX(ctx, key, value, ttl).Result()
}

// Delete elimina una key
func (d *Dragonfly) Delete(ctx context.Context, key string) error {
	return d.client.Del(ctx, key).Err()
}

// Exists verifica si una key existe
func (d *Dragonfly) Exists(ctx context.Context, key string) (bool, error) {
	n, err := d.client.Exists(ctx, key).Result()
	return n > 0, err
}

// TTL obtiene el TTL restante
func (d *Dragonfly) TTL(ctx context.Context, key string) (time.Duration, error) {
	return d.client.TTL(ctx, key).Result()
}

// Expire establece TTL para una key existente
func (d *Dragonfly) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return d.client.Expire(ctx, key, ttl).Err()
}

// =============================================================================
// Hash Operations (con soporte para HEXPIRE v1.38)
// =============================================================================

// HGet obtiene un campo de un hash
func (d *Dragonfly) HGet(ctx context.Context, key, field string) (string, error) {
	return d.client.HGet(ctx, key, field).Result()
}

// HSet establece campos en un hash
func (d *Dragonfly) HSet(ctx context.Context, key string, fields map[string]interface{}) error {
	return d.client.HSet(ctx, key, fields).Err()
}

// HGetAll obtiene todos los campos de un hash
func (d *Dragonfly) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return d.client.HGetAll(ctx, key).Result()
}

// HDel elimina campos de un hash
func (d *Dragonfly) HDel(ctx context.Context, key string, fields ...string) error {
	return d.client.HDel(ctx, key, fields...).Err()
}

// HEXPIRE establece TTL por campo individual (v1.38)
// Esto es más eficiente que aplicar TTL a toda la key
func (d *Dragonfly) HEXPIRE(ctx context.Context, key string, ttl time.Duration, fields ...string) error {
	if len(fields) == 0 {
		return nil
	}
	// HEXPIRE key seconds FIELDS N field [field...]
	args := []interface{}{"HEXPIRE", key, int(ttl.Seconds()), "FIELDS", len(fields)}
	for _, f := range fields {
		args = append(args, f)
	}
	_, err := d.client.Do(ctx, args...).Result()
	return err
}

// HTTL obtiene TTL de un campo específico (v1.38)
func (d *Dragonfly) HTTL(ctx context.Context, key, field string) (int64, error) {
	result, err := d.client.Do(ctx, "HTTL", key, "1", field).Int64Slice()
	if err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return -2, nil // Campo no existe
	}
	return result[0], nil
}

// =============================================================================
// Set Operations
// =============================================================================

// SAdd agrega miembros a un set
func (d *Dragonfly) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return d.client.SAdd(ctx, key, members...).Err()
}

// SMembers obtiene todos los miembros
func (d *Dragonfly) SMembers(ctx context.Context, key string) ([]string, error) {
	return d.client.SMembers(ctx, key).Result()
}

// SRem elimina miembros
func (d *Dragonfly) SRem(ctx context.Context, key string, members ...interface{}) error {
	return d.client.SRem(ctx, key, members...).Err()
}

// =============================================================================
// Counter Operations (para rate limiting)
// =============================================================================

// Incr incrementa un contador atómicamente
func (d *Dragonfly) Incr(ctx context.Context, key string) (int64, error) {
	return d.client.Incr(ctx, key).Result()
}

// Decr decrementa un contador
func (d *Dragonfly) Decr(ctx context.Context, key string) (int64, error) {
	return d.client.Decr(ctx, key).Result()
}

// =============================================================================
// Health & Lifecycle
// =============================================================================

// Ping verifica la conexión
func (d *Dragonfly) Ping(ctx context.Context) error {
	return d.client.Ping(ctx).Err()
}

// Close es un no-op. El cliente es compartido y propiedad de bootstrap.
// Cerrarlo aquí mataría la conexión para todos los módulos.
func (d *Dragonfly) Close() error {
	return nil
}

// =============================================================================
// Hashtag Utilities - CRÍTICO para evitar Global Lock en Lua scripts
// =============================================================================

// Convenção: usar {category}:identifier para que Dragonfly place las keys en el mismo shard
const (
	HashtagRateLimit = "{ratelimit}"
	HashtagSession   = "{session}"
	HashtagPerm      = "{perm}"
	HashtagGeoIP     = "{geoip}"
	HashtagSearch    = "{search}"
)

// RateLimitKey genera una key para rate limiting con hashtag correcto
func RateLimitKey(identifier string) string {
	return fmt.Sprintf("%s:%s", HashtagRateLimit, identifier)
}

// SessionKey genera una key para sesión
func SessionKey(userID string) string {
	return fmt.Sprintf("%s:user:%s", HashtagSession, userID)
}

// PermissionKey genera una key para permisos
func PermissionKey(userID string) string {
	return fmt.Sprintf("%s:%s", HashtagPerm, userID)
}
