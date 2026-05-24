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
	"strings"
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
	OpenWeatherTimeout  time.Duration
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
	Server             ServerConfig
	DB                 DBConfig
	Dragonfly          DragonflyConfig
	Frontend           FrontendConfig
	Security           SecurityConfig
	Email              EmailConfig
	Environment        EnvironmentConfig
	OAuth              OAuthConfig                // Credenciales OAuth por proveedor
	SerpAPIKey         string                     // API key de SerpAPI para búsqueda de vuelos
	PasetoKeyBytes     []byte                     // Bytes decodificados de PASETO_KEY (para uso directo)
	RateLimit          *ratelimit.RateLimitConfig // Configuración de rate limiting
	DefaultCurrency    string                     // Moneda por defecto para usuarios sin geoip (DEFAULT_CURRENCY env)
	DefaultLanguage    string                     // Idioma por defecto para usuarios sin geoip (DEFAULT_LANGUAGE env)
	DefaultCountryCode string                     // País por defecto para búsquedas sin geoip (DEFAULT_COUNTRY_CODE env)
	AI                 AIConfig                   // Configuración del intérprete AI (búsqueda NL)
	OCR                AIOCRConfig                // Configuración del OCR de documentos
	Medical            MedicalConfig              // Configuración del módulo médico
	Documents          DocumentLimitsConfig       // Límites de documentos
	R2                 R2StorageConfig            // Configuración de R2 (S3-compatible)
	CookieDomain       string                     // Dominio para cookies de auth (.proactrip.com en prod, vacío en dev)
	AvatarBaseURL      string                     // URL base para CDN de avatares (AVATAR_BASE_URL env, ej. "https://cdn.proactrip.com")
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
	Provider        string        // "deepseek" | "ollama" | "openai"
	BaseURL         string        // API endpoint base
	APIKey          string        // API key (omit for ollama local)
	Model           string        // e.g. "deepseek-v4-flash" for exact search
	DiscoveryModel  string        // model override for discovery (defaults to Model if empty)
	Timeout         time.Duration // timeout for AI requests
}

// AIOCRConfig contiene la configuración del servicio OCR para documentos.
// Usa DeepSeek V4 Flash multimodal para extracción de datos de documentos.
type AIOCRConfig struct {
	Provider string        // "deepseek" | "openai"
	BaseURL  string        // API endpoint base
	APIKey   string        // API key
	Model    string        // e.g. "deepseek-chat" (multimodal required)
	Timeout  time.Duration // timeout for OCR requests (longer than search)
}

// MedicalConfig contiene la configuración del módulo médico.
type MedicalConfig struct {
	EncryptionKey string // 32 bytes (64 hex chars) para ChaCha20-Poly1305
}

// EncryptionKeyBytes decodifica la clave de hex a bytes.
func (c *MedicalConfig) EncryptionKeyBytes() ([]byte, error) {
	if c.EncryptionKey == "" {
		return nil, nil // sin clave → sin encriptación
	}
	return hex.DecodeString(c.EncryptionKey)
}

// DocumentLimitsConfig contiene los límites de subida de documentos.
type DocumentLimitsConfig struct {
	MaxSizeMB  int // Tamaño máximo por documento (MB)
	ImageMaxMB int // Tamaño máximo para imágenes (MB)
	MaxPerUser int // Máximo de documentos por usuario
	RateLimit  int // Subidas por minuto por usuario
	RateWindow int // Ventana del rate limit (segundos)
}

// R2StorageConfig contiene la configuración de R2 (S3-compatible).
type R2StorageConfig struct {
	Endpoint  string // R2 endpoint (ej: https://account.r2.cloudflarestorage.com)
	AccessKey string // R2 access key ID
	SecretKey string // R2 secret access key
	Bucket    string // R2 bucket name
	UseSSL    bool   // Usar HTTPS para conexiones
}

// IsConfigured verifica si R2 está configurado.
func (c *R2StorageConfig) IsConfigured() bool {
	return c.Endpoint != "" && c.AccessKey != "" && c.SecretKey != ""
}

// ValidateAll valida todas las configuraciones sensibles al arrancar.
// Bloquea el startup (error) en claves críticas faltantes/inválidas.
// Advierte (warning) en claves opcionales pero importantes.
func ValidateAll(cfg *Config) error {
	if err := ValidateSecureConfig(cfg); err != nil {
		return err
	}
	if err := ValidateDatabaseConfig(cfg); err != nil {
		return err
	}
	if err := validateAIConfig(cfg); err != nil {
		return err
	}
	if err := validateRateLimitConfig(cfg); err != nil {
		return err
	}
	ValidateOptionalSecrets(cfg)
	return nil
}

// validateAIConfig checks AI provider config sanity.
func validateAIConfig(cfg *Config) error {
	if cfg.AI.Provider == "" {
		return nil // AI not configured — optional feature
	}
	if cfg.AI.Timeout <= 0 {
		return errors.New("AI_SEARCH_TIMEOUT must be > 0 when AI provider is configured")
	}
	return nil
}

// validateRateLimitConfig checks rate limit tier ranges are sane.
func validateRateLimitConfig(cfg *Config) error {
	if cfg.RateLimit == nil {
		return nil
	}
	if cfg.RateLimit.GlobalPerMinute <= 0 {
		return errors.New("RATELIMIT_GLOBAL_PER_MINUTE must be > 0")
	}
	if cfg.RateLimit.AuthenticatedPerMinute <= 0 {
		return errors.New("RATELIMIT_AUTH_PER_MINUTE must be > 0")
	}
	if cfg.RateLimit.AnonymousPerMinute <= 0 {
		return errors.New("RATELIMIT_ANON_PER_MINUTE must be > 0")
	}
	if cfg.RateLimit.AuthenticatedPerMinute > cfg.RateLimit.GlobalPerMinute {
		return errors.New("RATELIMIT_AUTH_PER_MINUTE must be <= RATELIMIT_GLOBAL_PER_MINUTE")
	}
	if cfg.RateLimit.AnonymousPerMinute > cfg.RateLimit.AuthenticatedPerMinute {
		return errors.New("RATELIMIT_ANON_PER_MINUTE must be <= RATELIMIT_AUTH_PER_MINUTE")
	}
	return nil
}

// ValidateDatabaseConfig valida que las credenciales de base de datos estén configuradas.
func ValidateDatabaseConfig(cfg *Config) error {
	if cfg.DB.Password == "" {
		return errors.New("DB_PASSWORD es requerida para producción")
	}
	if cfg.Dragonfly.Password == "" {
		return errors.New("DRAGONFLY_PASSWORD es requerida para producción")
	}
	return nil
}

// ValidateOptionalSecrets advierte sobre claves opcionales no configuradas.
// No bloquea el startup — son funcionalidades que pueden estar deshabilitadas.
func ValidateOptionalSecrets(cfg *Config) {
	if cfg.Email.ResendAPIKey == "" {
		fmt.Println("⚠️  WARNING: RESEND_API_KEY no configurada. Los emails no se enviarán.")
	}
	if cfg.OAuth.GoogleClientSecret == "" {
		fmt.Println("⚠️  WARNING: GOOGLE_CLIENT_SECRET no configurada. OAuth de Google deshabilitado.")
	}
	if cfg.OAuth.GoogleClientID == "" {
		fmt.Println("⚠️  WARNING: GOOGLE_CLIENT_ID no configurada. OAuth de Google deshabilitado.")
	}
	if cfg.SerpAPIKey == "" {
		fmt.Println("⚠️  WARNING: SERPAPI_KEY no configurada. Las búsquedas de vuelos no funcionarán.")
	}
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
			OpenWeatherCacheTTL: getEnvDuration("ENVIRONMENT_WEATHER_CACHE_TTL", 30*time.Minute),
			OpenWeatherTimeout:  getEnvDuration("OPENWEATHER_TIMEOUT", 10*time.Second),
			IpQueryBaseURL:      cmp.Or(getEnv("IPQUERY_BASE_URL", ""), "https://api.ipquery.io"),
		},
		OAuth: OAuthConfig{
			GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", ""),
		},
		SerpAPIKey:         getEnv("SERPAPI_KEY", ""),
		RateLimit:          ratelimit.LoadRateLimitConfig(),
		DefaultCurrency:    getEnv("DEFAULT_CURRENCY", "EUR"),
		DefaultLanguage:    getEnv("DEFAULT_LANGUAGE", "es"),
		DefaultCountryCode: getEnv("DEFAULT_COUNTRY_CODE", "ES"),
		AI: AIConfig{
			Provider:       getEnv("AI_SEARCH_PROVIDER", ""),
			BaseURL:        getEnv("AI_SEARCH_BASE_URL", ""),
			APIKey:         getEnv("AI_SEARCH_API_KEY", ""),
			Model:          getEnv("AI_SEARCH_MODEL", ""),
			DiscoveryModel: getEnv("AI_SEARCH_DISCOVERY_MODEL", ""),
			Timeout:        getEnvDuration("AI_SEARCH_TIMEOUT", 30*time.Second),
		},
		OCR: AIOCRConfig{
			Provider: getEnv("AI_OCR_PROVIDER", ""),
			BaseURL:  getEnv("AI_OCR_BASE_URL", ""),
			APIKey:   getEnv("AI_OCR_API_KEY", ""),
			Model:    getEnv("AI_OCR_MODEL", "deepseek-v4-flash"),
			Timeout:  getEnvDuration("AI_OCR_TIMEOUT", 60*time.Second),
		},
		Medical: MedicalConfig{
			EncryptionKey: getEnv("MEDICAL_ENCRYPTION_KEY", ""),
		},
		Documents: DocumentLimitsConfig{
			MaxSizeMB:  getEnvInt("DOCUMENT_MAX_SIZE_MB", 20),
			ImageMaxMB: getEnvInt("IMAGE_MAX_SIZE_MB", 10),
			MaxPerUser: getEnvInt("DOCUMENT_MAX_PER_USER", 5),
			RateLimit:  getEnvInt("DOCUMENT_UPLOAD_RATE_LIMIT", 10),
			RateWindow: getEnvInt("DOCUMENT_UPLOAD_RATE_WINDOW", 60),
		},
		R2: R2StorageConfig{
			Endpoint:  getEnv("R2_ENDPOINT", ""),
			AccessKey: getEnv("R2_ACCESS_KEY_ID", ""),
			SecretKey: getEnv("R2_SECRET_ACCESS_KEY", ""),
			Bucket:    getEnv("R2_BUCKET_NAME", "proactrip"),
			UseSSL:    getEnv("R2_USE_SSL", "true") == "true",
		},
		CookieDomain: getEnv("COOKIE_DOMAIN", func() string {
			if getEnv("SERVER_ENV", "dev") == "prod" {
				return ".proactrip.com"
			}
			return ""
		}()),
		AvatarBaseURL: getEnv("AVATAR_BASE_URL", ""),
	}
}

// getEnv obtiene valor de variable de entorno, retorna default si no existe.
// Soporta comentarios inline con # (formato .env de Docker).
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	// Strip inline comments (e.g., "value  # comment")
	if idx := strings.IndexByte(value, '#'); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
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

func getEnvInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return n
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
