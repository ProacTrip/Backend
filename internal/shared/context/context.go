package contextutil

// =============================================================================
// Context Keys - Typed keys para evitar colisiones (WARN: string key collisions)
// =============================================================================

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type contextKey string

const (
	// TraceIDKey es la clave type-safe para el trace ID
	TraceIDKey contextKey = "trace_id"

	// RequestIDKey es la clave type-safe para el request ID de Echo
	RequestIDKey contextKey = "request_id"
)

// =============================================================================
// Trace ID - Implementa W3C traceparent specification
// =============================================================================

// TraceID representa un trace ID tipo W3C traceparent
// Formato: 00-{trace-id}-{parent-span-id}-01
type TraceID struct {
	TraceID      string // 32 hex chars (128 bits)
	ParentSpanID string // 16 hex chars (64 bits)
	TraceFlags   string // 01 = sampled
}

// NewTraceID crea un nuevo TraceID con formato W3C traceparent
func NewTraceID() *TraceID {
	// Generar 16 bytes aleatorios para trace-id (128 bits)
	traceBytes := make([]byte, 16)
	rand.Read(traceBytes)
	traceID := hex.EncodeToString(traceBytes)

	// Generar 8 bytes aleatorios para parent-span-id (64 bits)
	parentBytes := make([]byte, 8)
	rand.Read(parentBytes)
	parentSpanID := hex.EncodeToString(parentBytes)

	return &TraceID{
		TraceID:      traceID,
		ParentSpanID: parentSpanID,
		TraceFlags:   "01",
	}
}

// Traceparent retorna el header W3C traceparent
func (t *TraceID) Traceparent() string {
	return "00-" + t.TraceID + "-" + t.ParentSpanID + "-" + t.TraceFlags
}

// =============================================================================
// Context Helpers - Funciones type-safe para trabajar con contexto
// =============================================================================

// WithTraceID agrega un trace ID al contexto
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GetTraceID extrae el trace ID del contexto de forma type-safe
func GetTraceID(ctx context.Context) string {
	if v := ctx.Value(TraceIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithRequestID agrega un request ID al contexto
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID extrae el request ID del contexto de forma type-safe
func GetRequestID(ctx context.Context) string {
	if v := ctx.Value(RequestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// =============================================================================
// Request Context - Agrega trace ID al request si no existe
// =============================================================================

// EnsureTraceID asegura que el contexto tenga un trace ID
// Si ya existe, lo retorna; si no, genera uno nuevo
func EnsureTraceID(ctx context.Context) context.Context {
	if existing := GetTraceID(ctx); existing != "" {
		return ctx
	}

	traceID := NewTraceID()
	return WithTraceID(ctx, traceID.TraceID)
}


