package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/v2"
)

// Configuración raíz
type Config struct {
	App      AppConfig      `koanf:"app"`
	Server   ServerConfig   `koanf:"server"`
	Database DatabaseConfig `koanf:"database"`
	Cache    CacheConfig    `koanf:"cache"`
	EventBus EventBusConfig `koanf:"eventbus"`
}

// AppConfig
type AppConfig struct {
	Name        string `koanf:"name"`
	Version     string `koanf:"version"`
	Environment string `koanf:"environment"`
	Debug       bool   `koanf:"debug"`
	FrontendURL string `koanf:"frontend_url"`
}

// ServerConfig
type ServerConfig struct {
	Port            string        `koanf:"port"`
	ReadTimeout     time.Duration `koanf:"read_timeout"`
	WriteTimeout    time.Duration `koanf:"write_timeout"`
	IdleTimeout     time.Duration `koanf:"idle_timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
}

// RedisConfig
type CacheConfig struct {
	URL      string `koanf:"url"`
	PoolSize int    `koanf:"pool_size"`
}

// DatabaseConfig
type DatabaseConfig struct {
	Auth            string        `koanf:"auth"`
	Reference       string        `koanf:"reference"`
	User            string        `koanf:"user"`
	Booking         string        `koanf:"booking"`
	Payment         string        `koanf:"payment"`
	Search          string        `koanf:"search"`
	Notification    string        `koanf:"notification"`
	Audit           string        `koanf:"audit"`
	MaxOpenConns    int           `koanf:"max_open_conns"`
	MaxIdleConns    int           `koanf:"max_idle_conns"`
	MaxConnLifetime time.Duration `koanf:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `koanf:"max_conn_idletime"`
}

// EventBusConfig
type EventBusConfig struct {
	URL          string        `koanf:"url"`
	PoolSize     int           `koanf:"pool_size"`
	Auth         StreamConfig  `koanf:"auth"`
	Reference    StreamConfig  `koanf:"reference"`
	User         StreamConfig  `koanf:"user"`
	Booking      StreamConfig  `koanf:"booking"`
	Payment      StreamConfig  `koanf:"payment"`
	Search       StreamConfig  `koanf:"search"`
	Notification StreamConfig  `koanf:"notification"`
	Audit        StreamConfig  `koanf:"audit"`
	MaxLen       int64         `koanf:"max_len"`
	BlockTime    time.Duration `koanf:"block_time"`
}

type StreamConfig struct {
	StreamName string `koanf:"stream_name"`
	GroupName  string `koanf:"group_name"`
	// ConsumerName eliminado intencionalmente. Se generará en runtime (ej: os.Hostname())
}

// Load — Carga configuración desde variables de entorno
func Load() (*Config, error) {
	k := koanf.New(".")

	err := k.Load(env.Provider(".", env.Opt{
		TransformFunc: func(key, val string) (string, any) {
			s := strings.ToLower(key)

			// 1. APP
			if trimmed, ok := strings.CutPrefix(s, "app_"); ok {
				return "app." + trimmed, val
			}

			// 2. SERVER
			if trimmed, ok := strings.CutPrefix(s, "server_"); ok {
				return "server." + trimmed, val
			}

			// 3. DATABASES
			if name, ok := strings.CutSuffix(s, "_database_url"); ok {
				return "database." + name, val
			}

			if strings.HasPrefix(s, "db_max_") {
				trimmed := strings.TrimPrefix(s, "db_")
				return "database." + trimmed, val
			}

			// 4. EVENTBUS
			if trimmed, ok := strings.CutPrefix(s, "eventbus_"); ok {
				return "eventbus." + trimmed, val
			}

			// 5. CACHE
			if trimmed, ok := strings.CutPrefix(s, "cache_"); ok {
				return "cache." + trimmed, val
			}

			return key, val
		},
	}), nil)

	if err != nil {
		return nil, fmt.Errorf("error cargando variables de entorno: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("error parseando configuración: %w", err)
	}

	return &cfg, nil
}
