// Configuración centralizada de la aplicación.
// Carga valores desde variables de entorno y provee estructuras para cada componente.
package config

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInvalidPasetoKey indica que la clave PASETO no es válida
var ErrInvalidPasetoKey = errors.New("PASETO_KEY debe tener exactamente 32 bytes (64 caracteres hex)")

// generateSecureKey genera una clave de 32 bytes en formato hex
func generateSecureKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generar clave: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

type EnvironmentConfig struct {
	OpenWeatherAPIKey   string
	OpenWeatherCacheTTL time.Duration
	IpQueryBaseURL      string
}

// OAuthConfig contiene las credenciales OAuth por proveedor.
// Google → backend → 302 redirect al frontend.
type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string // URL donde Google redirige después de autorizar (backend callback)
}

// Configuración principal que agrupa todos los componentes de la aplicación
type Config struct {
	Server         ServerConfig
	DB             DBConfig
	Dragonfly      DragonflyConfig
	Frontend       FrontendConfig
	Security       SecurityConfig
	Email          EmailConfig
	Environment    EnvironmentConfig
	OAuth          OAuthConfig               // Credenciales OAuth por proveedor
	SerpAPIKey     string                    // API key de SerpAPI para búsqueda de vuelos
	PasetoKeyBytes []byte                    // Bytes decodificados de PASETO_KEY (para uso directo)
	RateLimit      *ratelimit.RateLimitConfig // Configuración de rate limiting
	DefaultCurrency   string                 // Moneda por defecto para usuarios sin geoip (DEFAULT_CURRENCY env)
	DefaultLanguage   string                 // Idioma por defecto para usuarios sin geoip (DEFAULT_LANGUAGE env)
	DefaultCountryCode string               // País por defecto para búsquedas sin geoip (DEFAULT_COUNTRY_CODE env)
	AI                AIConfig               // Configuración del intérprete AI
}

// Configuración de la base de datos PostgreSQL
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// DSN genera el string de conexión para la base de datos
func (c *DBConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)
}

// GetPool returns a pgxpool.Pool for the database (legacy single DB)
func (c *DBConfig) GetPool() (*pgxpool.Pool, error) {
	return pgxpool.New(context.Background(), c.DSN())
}

// GetDSNForDB returns DSN for a specific database name (multi-tenant per module)
func (c *DBConfig) GetDSNForDB(dbName string) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, dbName, c.SSLMode)
}

// Configuración del servidor HTTP
type ServerConfig struct {
	Port string
	Env  string
}

// Configuración de DragonflyDB (cache)
type DragonflyConfig struct {
	Host     string
	Port     string
	Password string
}

// URLs del frontend para desarrollo y producción
type FrontendConfig struct {
	DevURL  string
	ProdURL string
}

// Configuración de seguridad
type SecurityConfig struct {
	PasetoKey string
}

// Configuración de email (Resend API)
type EmailConfig struct {
	ResendAPIKey string
}

// AIConfig contiene la configuración del intérprete de lenguaje natural.
// Soporta múltiples proveedores (deepseek, ollama, openai, etc.).
type AIConfig struct {
	Provider string        // "deepseek" | "ollama" | "openai"
	BaseURL  string        // API endpoint base
	APIKey   string        // API key (omit for ollama local)
	Model    string        // e.g. "deepseek-chat", "dolphin-mistral"
	Timeout  time.Duration // timeout for AI requests
}

// ValidateSecureConfig valida las claves criptográficas al arrancar
// RETORNA ERROR si PASETO_KEY no existe o no tiene 64 caracteres hex (32 bytes)
// IMPORTANTE: Convierte la clave hex a bytes para uso directo en PASETO
func ValidateSecureConfig(cfg *Config) error {
	if cfg.Security.PasetoKey == "" {
		key, err := generateSecureKey()
		if err != nil {
			return fmt.Errorf("generar PASETO_KEY: %w", err)
		}
		fmt.Printf("⚠️  WARNING: PASETO_KEY no configurada. Generada clave temporal:\n%s\n", key)
		fmt.Println("⚠️  IMPORTANTE: Guarda esta clave en .env antes de producción!")
		cfg.Security.PasetoKey = key
		// Decodificar hex a bytes para uso inmediato
		keyBytes, _ := hex.DecodeString(key)
		cfg.PasetoKeyBytes = keyBytes
		return nil
	}

	// Validar que sea exactamente 64 hex chars (32 bytes)
	if len(cfg.Security.PasetoKey) != 64 {
		return fmt.Errorf("%w: longitud %d (esperado 64 hex chars)", ErrInvalidPasetoKey, len(cfg.Security.PasetoKey))
	}

	// Validar que sea válido hex
	keyBytes, err := hex.DecodeString(cfg.Security.PasetoKey)
	if err != nil {
		return fmt.Errorf("%w: no es hex válido", ErrInvalidPasetoKey)
	}

	// Guardar los bytes decodificados para uso directo en PASETO
	cfg.PasetoKeyBytes = keyBytes

	return nil
}

// Load carga la configuración desde variables de entorno
// WARNING: Después de Load() debe llamarse ValidateSecureConfig()
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
			Env:  getEnv("SERVER_ENV", "dev"),
		},
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", "proactrip"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Dragonfly: DragonflyConfig{
			Host:     getEnv("DRAGONFLY_HOST", "localhost"),
			Port:     getEnv("DRAGONFLY_PORT", "6379"),
			Password: getEnv("DRAGONFLY_PASSWORD", ""),
		},
		Frontend: FrontendConfig{
			DevURL:  getEnv("FRONTEND_URL_DEV", "http://localhost:3000"),
			ProdURL: getEnv("FRONTEND_URL_PROD", "https://proactrip.com"),
		},
		Security: SecurityConfig{
			PasetoKey: getEnv("PASETO_KEY", ""), // Sin default - validar después
		},
		Email: EmailConfig{
			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
		},
		Environment: EnvironmentConfig{
			OpenWeatherAPIKey:   getEnv("OPENWEATHER_API_KEY", ""),
			OpenWeatherCacheTTL: getEnvDuration("ENVIRONMENT_WEATHER_CACHE_TTL", 10*time.Minute),
			IpQueryBaseURL:      cmp.Or(getEnv("IPQUERY_BASE_URL", ""), "https://api.ipquery.io"),
		},
		OAuth: OAuthConfig{
			GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", ""),
		},
		SerpAPIKey: getEnv("SERPAPI_KEY", ""),
		RateLimit:       ratelimit.LoadRateLimitConfig(),
		DefaultCurrency:    getEnv("DEFAULT_CURRENCY", "EUR"),
		DefaultLanguage:    getEnv("DEFAULT_LANGUAGE", "es"),
		DefaultCountryCode: getEnv("DEFAULT_COUNTRY_CODE", "AR"),
		AI: AIConfig{
			Provider: getEnv("AI_PROVIDER", ""),
			BaseURL:  getEnv("AI_BASE_URL", ""),
			APIKey:   getEnv("AI_API_KEY", ""),
			Model:    getEnv("AI_MODEL", ""),
			Timeout:  getEnvDuration("AI_TIMEOUT", 30*time.Second),
		},
	}
}

// getEnv obtiene valor de variable de entorno, retorna default si no existe
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultValue
	}
	return d
}

// Addr retorna dirección del servidor en formato host:port
func (c *DragonflyConfig) Addr() string {
	return c.Host + ":" + c.Port
}

// IsProd verifica si el entorno es producción
func (c *ServerConfig) IsProd() bool {
	return c.Env == "prod"
}

// GetURL retorna la URL del frontend según el entorno
func (c *FrontendConfig) GetURL() string {
	if getEnv("SERVER_ENV", "dev") == "prod" {
		return c.ProdURL
	}
	return c.DevURL
}

// ParsePort convierte el puerto de string a int con fallback a 8080
func (c *ServerConfig) ParsePort() int {
	port, err := strconv.Atoi(c.Port)
	if err != nil {
		return 8080
	}
	return port
}

// Addr retorna la dirección del servidor en formato :puerto
func (c *ServerConfig) Addr() string {
	return ":" + c.Port
}
