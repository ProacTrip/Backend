package database

// =============================================================================
// Tipos de bases de datos y configuración de pools
// Cada DBType representa un bounded context del dominio
// =============================================================================

import "time"

// DBType representa cada bounded context / base de datos lógica
type DBType string

const (
	DBAuth         DBType = "auth"
	DBUser         DBType = "user"
	DBBooking      DBType = "booking"
	DBPayment      DBType = "payment"
	DBSearch       DBType = "search"
	DBNotification DBType = "notification"
	DBAudit        DBType = "audit"
)

// DBConfig contiene la configuración para un pool de base de datos
type DBConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}
