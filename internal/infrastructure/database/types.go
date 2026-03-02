package database

// DBType representa cada bounded context / base de datos lógica
type DBType string

const (
	DBAuth         DBType = "auth"
	DBReference    DBType = "reference"
	DBUser         DBType = "user"
	DBBooking      DBType = "booking"
	DBPayment      DBType = "payment"
	DBSearch       DBType = "search"
	DBNotification DBType = "notification"
	DBAudit        DBType = "audit"
)
