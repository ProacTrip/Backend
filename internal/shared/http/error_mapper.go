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
// Formato: RFC 9457 Problem Details
// Headers: X-Trace-Id, traceparent
// =============================================================================

// MapHTTPErrorToProblem convierte un echo.HTTPError a un Problem RFC 9457.
// Extraída como función compartida para eliminar duplicación entre error_mapper.go y app.go.
func MapHTTPErrorToProblem(he *echo.HTTPError, instance, traceID string) *serrors.Problem {
	problem := &serrors.Problem{
		Type:    serrors.ProblemTypeBadRequest,
		Title:   "Bad Request",
		Status:  he.Code,
		Detail:  he.Message,
		TraceID: traceID,
		Instance: instance,
	}

	switch he.Code {
	case http.StatusNotFound:
		problem.Type = serrors.ProblemTypeNotFound
		problem.Title = "Not Found"
	case http.StatusMethodNotAllowed:
		problem.Type = serrors.ProblemTypeBadRequest
		problem.Title = "Method Not Allowed"
	case http.StatusBadRequest:
		problem.Type = serrors.ProblemTypeValidationError
		problem.Title = "Validation Error"
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

	return problem
}

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
		return c.JSON(sp.Status, sp)
	}

	// 2. Check for echo.HTTPError y convertir a RFC 9457
	if he, ok := errors.AsType[*echo.HTTPError](err); ok {
		traceID := GetOrGenerateTraceID(c)

		problem := MapHTTPErrorToProblem(he, c.Request().URL.Path, traceID)

		c.Response().Header().Set("X-Trace-Id", traceID)
		c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
		return c.JSON(he.Code, problem)
	}

	// 3. Check for registered domain error mappers (registry pattern)
	if problem := serrors.MapDomainError(err); problem != nil {
		c.Response().Header().Set("X-Trace-Id", problem.TraceID)
		c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
		return c.JSON(problem.Status, problem)
	}

	// Fallback: generic internal error con trace_id
	traceID := GetOrGenerateTraceID(c)
	c.Response().Header().Set("X-Trace-Id", traceID)
	c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
	return c.JSON(http.StatusInternalServerError, &serrors.Problem{
		Type:    serrors.ProblemTypeInternalError,
		Title:   "Internal Server Error",
		Status:  http.StatusInternalServerError,
		Detail:  "An unexpected error occurred",
		TraceID: traceID,
	})
}

// GetOrGenerateTraceID reutiliza el trace ID existente del middleware si ya fue seteado.
// Solo genera un nuevo UUID v7 como fallback cuando no hay ninguno en los headers.
func GetOrGenerateTraceID(c *echo.Context) string {
	if traceID := c.Response().Header().Get("X-Trace-Id"); traceID != "" {
		return traceID
	}
	if traceID := c.Response().Header().Get(echo.HeaderXRequestID); traceID != "" {
		return traceID
	}
	return uuid.Must(uuid.NewV7()).String()
}
