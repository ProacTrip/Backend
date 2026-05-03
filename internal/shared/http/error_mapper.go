package http

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

// =============================================================================
// Error Mapper - Mapea errores a respuestas HTTP
// Formato: RFC 7807 Problem Details
// Headers: X-Trace-Id, traceparent
// =============================================================================

// MapError convierte errores a HTTP responses con formato RFC 9457.
// Maneja tanto errores del domain como errores ya convertidos a Problems.
func MapError(c *echo.Context, err error) error {
	if err == nil {
		return nil
	}

	// 1. Check for shared Problem (RFC 9457) - ya convertido
	var sp *serrors.Problem
	if errors.As(err, &sp) {
		c.Response().Header().Set("X-Trace-Id", sp.TraceID)
		c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
		setTraceparentFromContext(c)
		return c.JSON(sp.Status, sp)
	}

	// 2. Check for echo.HTTPError y convertir a RFC 9457
	if he, ok := errors.AsType[*echo.HTTPError](err); ok {
		traceID := uuid.Must(uuid.NewV7()).String()
		c.Response().Header().Set("X-Trace-Id", traceID)
		c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
		setTraceparentFromContext(c)

		// Message es un string en Echo v5
		msg := he.Message

		problem := &serrors.Problem{
			Type:    serrors.ProblemTypeBadRequest,
			Title:   "Bad Request",
			Status:  he.Code,
			Detail:  msg,
			TraceID: traceID,
		}

		// Ajustar tipo según código
		switch he.Code {
		case http.StatusNotFound:
			problem.Type = serrors.ProblemTypeNotFound
			problem.Title = "Not Found"
		case http.StatusMethodNotAllowed:
			problem.Type = serrors.ProblemTypeBadRequest
			problem.Title = "Method Not Allowed"
		case http.StatusBadRequest:
			problem.Type = serrors.ProblemTypeBadRequest
			problem.Title = "Bad Request"
		case http.StatusUnauthorized:
			problem.Type = serrors.ProblemTypeUnauthorized
			problem.Title = "Unauthorized"
		case http.StatusForbidden:
			problem.Type = serrors.ProblemTypeForbidden
			problem.Title = "Forbidden"
		case http.StatusTooManyRequests:
			problem.Type = serrors.ProblemTypeTooManyRequests
			problem.Title = "Too Many Requests"
		}

		return c.JSON(he.Code, problem)
	}

	// 3. Check for registered domain error mappers (registry pattern)
	if problem := serrors.MapDomainError(err); problem != nil {
		c.Response().Header().Set("X-Trace-Id", problem.TraceID)
		c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
		setTraceparentFromContext(c)
		return c.JSON(problem.Status, problem)
	}

	// Fallback: generic internal error con trace_id
	traceID := uuid.Must(uuid.NewV7()).String()
	c.Response().Header().Set("X-Trace-Id", traceID)
	c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
	setTraceparentFromContext(c)
	return c.JSON(http.StatusInternalServerError, &serrors.Problem{
		Type:    serrors.ProblemTypeInternalError,
		Title:   "Internal Server Error",
		Status:  http.StatusInternalServerError,
		Detail:  "An unexpected error occurred",
		TraceID: traceID,
	})
}

// setTraceparentFromContext extrae el traceparent del contexto si existe
func setTraceparentFromContext(c *echo.Context) {
	if tp := c.Response().Header().Get("traceparent"); tp != "" {
		// Ya está seteado por el middleware
		return
	}
	// El middleware de app.go ya debería setearlo
}
