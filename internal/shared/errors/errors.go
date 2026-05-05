package errors

// =============================================================================
// RFC 9457 Problem Details - Formato estándar de errores HTTP
// https://www.rfc-editor.org/rfc/rfc9457
// =============================================================================

import (
	"net/http"

	"github.com/google/uuid"
)

// ProblemType identifica categorías de errores como URIs absolutas (RFC 9457)
type ProblemType string

const (
	// 4xx Client Errors
	ProblemTypeBadRequest      ProblemType = "https://api.proactrip.com/errors/bad-request"
	ProblemTypeUnauthorized    ProblemType = "https://api.proactrip.com/errors/unauthorized"
	ProblemTypeForbidden       ProblemType = "https://api.proactrip.com/errors/forbidden"
	ProblemTypeNotFound        ProblemType = "https://api.proactrip.com/errors/not-found"
	ProblemTypeConflict        ProblemType = "https://api.proactrip.com/errors/conflict"
	ProblemTypeValidationError ProblemType = "https://api.proactrip.com/errors/validation-error"
	ProblemTypeTooManyRequests ProblemType = "https://api.proactrip.com/errors/rate-limit-exceeded"

	// 5xx Server Errors
	ProblemTypeInternalError      ProblemType = "https://api.proactrip.com/errors/internal-error"
	ProblemTypeBadGateway         ProblemType = "https://api.proactrip.com/errors/bad-gateway"
	ProblemTypeServiceUnavailable ProblemType = "https://api.proactrip.com/errors/service-unavailable"
)

// Problem es el formato RFC 9457 Problem Details
type Problem struct {
	Type     ProblemType `json:"type"`               // Identificador único del tipo de problema
	Title    string      `json:"title"`              // Título breve legible humano
	Status   int         `json:"status"`             // Código de estado HTTP
	Detail   string      `json:"detail"`             // Detail específico sobre el problema
	Instance string      `json:"instance,omitempty"` // Path del endpoint que originó el error
	TraceID  string      `json:"trace_id,omitempty"` // UUID v7 para trazabilidad

	// Err underlying error (no se serializa, solo para logging)
	Err error `json:"-"`
}

// Error implementa error interface
func (p *Problem) Error() string {
	return p.Detail
}

// Unwrap retorna el error subyacente para errors.Is/As
func (p *Problem) Unwrap() error {
	return p.Err
}

// Is implementa error interface para errors.Is() — permite comparar por tipo
func (p *Problem) Is(target error) bool {
	if target == nil {
		return false
	}
	// Comparar con otro Problem
	if other, ok := target.(*Problem); ok {
		return p.Type == other.Type
	}
	return false
}

// New crea un nuevo Problem con los campos requeridos
func New(typ ProblemType, title, detail string, httpStatus int, err error) *Problem {
	return &Problem{
		Type:    typ,
		Title:   title,
		Status:  httpStatus,
		Detail:  detail,
		Err:     err,
		TraceID: uuid.Must(uuid.NewV7()).String(),
	}
}

// WithInstance agrega el path del endpoint
func (p *Problem) WithInstance(path string) *Problem {
	p.Instance = path
	return p
}

// =============================================================================
// Constructor helpers - usando Go 1.26 new(expr) pattern
// Funciones factory para crear errores rápidamente
// =============================================================================

var (
	ErrBadRequest = func(detail string, err error) *Problem {
		return New(ProblemTypeBadRequest, "Bad Request", detail, http.StatusBadRequest, err)
	}
	ErrUnauthorized = func(detail string, err error) *Problem {
		return New(ProblemTypeUnauthorized, "Unauthorized", detail, http.StatusUnauthorized, err)
	}
	ErrForbidden = func(detail string, err error) *Problem {
		return New(ProblemTypeForbidden, "Forbidden", detail, http.StatusForbidden, err)
	}
	ErrNotFound = func(detail string, err error) *Problem {
		return New(ProblemTypeNotFound, "Not Found", detail, http.StatusNotFound, err)
	}
	ErrConflict = func(detail string, err error) *Problem {
		return New(ProblemTypeConflict, "Conflict", detail, http.StatusConflict, err)
	}
	ErrValidationError = func(detail string, err error) *Problem {
		return New(ProblemTypeValidationError, "Validation Error", detail, http.StatusBadRequest, err)
	}
	ErrTooManyRequests = func(detail string, err error) *Problem {
		return New(ProblemTypeTooManyRequests, "Too Many Requests", detail, http.StatusTooManyRequests, err)
	}
	ErrInternalError = func(detail string, err error) *Problem {
		return New(ProblemTypeInternalError, "Internal Server Error", detail, http.StatusInternalServerError, err)
	}
	ErrBadGateway = func(detail string, err error) *Problem {
		return New(ProblemTypeBadGateway, "Bad Gateway", detail, http.StatusBadGateway, err)
	}
	ErrServiceUnavailable = func(detail string, err error) *Problem {
		return New(ProblemTypeServiceUnavailable, "Service Unavailable", detail, http.StatusServiceUnavailable, err)
	}
)

// =============================================================================
// Domain Error Mapper Registry - Permite a los módulos registrar mapeos
// de sus errores de dominio sin que el paquete shared/http importe módulos específicos
// =============================================================================

// DomainErrorMapper mapea un error de dominio específico a un Problem RFC 9457.
// Retorna nil si el mapper no reconoce el error.
type DomainErrorMapper func(error) *Problem

var domainMappers []DomainErrorMapper

// RegisterDomainErrorMapper registra un mapper de errores de dominio.
// Los módulos deben llamarlo en sus funciones init().
func RegisterDomainErrorMapper(m DomainErrorMapper) {
	domainMappers = append(domainMappers, m)
}

// MapDomainError itera sobre todos los mappers registrados para encontrar
// un Problem correspondiente al error de dominio. Retorna nil si no se encuentra.
func MapDomainError(err error) *Problem {
	for _, m := range domainMappers {
		if p := m(err); p != nil {
			return p
		}
	}
	return nil
}

// HTTPStatusFromType mapea ProblemType a código HTTP
func HTTPStatusFromType(typ ProblemType) int {
	switch typ {
	case ProblemTypeBadRequest, ProblemTypeValidationError:
		return http.StatusBadRequest
	case ProblemTypeUnauthorized:
		return http.StatusUnauthorized
	case ProblemTypeForbidden:
		return http.StatusForbidden
	case ProblemTypeNotFound:
		return http.StatusNotFound
	case ProblemTypeConflict:
		return http.StatusConflict
	case ProblemTypeTooManyRequests:
		return http.StatusTooManyRequests
	case ProblemTypeInternalError:
		return http.StatusInternalServerError
	case ProblemTypeBadGateway:
		return http.StatusBadGateway
	case ProblemTypeServiceUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
