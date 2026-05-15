// Tests de observabilidad para el pipeline de discovery.
// Verifica que los logs estructurados se emiten con los key-value pairs correctos.
package ai_search

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// =============================================================================
// Helpers — captura de logs en tests
// =============================================================================

// capturingHandler captura los registros de slog en un buffer para verificación.
type capturingHandler struct {
	buf    bytes.Buffer
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

func (h *capturingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *capturingHandler) Handle(ctx context.Context, r slog.Record) error {
	// Escribir como JSON para fácil verificación
	enc := json.NewEncoder(&h.buf)
	m := map[string]any{
		"msg":   r.Message,
		"level": r.Level.String(),
	}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.String()
		return true
	})
	_ = enc.Encode(m)
	return nil
}

func (h *capturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &capturingHandler{
		level:  h.level,
		attrs:  append(h.attrs, attrs...),
		groups: h.groups,
	}
}

func (h *capturingHandler) WithGroup(name string) slog.Handler {
	return &capturingHandler{
		level:  h.level,
		attrs:  h.attrs,
		groups: append(h.groups, name),
	}
}

// capturedLogs returns the captured log output as a string.
func (h *capturingHandler) String() string {
	return h.buf.String()
}

// reset clears captured logs.
func (h *capturingHandler) reset() {
	h.buf.Reset()
}

// hasKeyValue checks if captured logs contain a key with a specific value.
func (h *capturingHandler) hasKeyValue(key, value string) bool {
	return strings.Contains(h.buf.String(), `"`+key+`":"`+value+`"`)
}

// =============================================================================
// LogDiscoveryRequest test
// =============================================================================

func TestLogDiscoveryRequest(t *testing.T) {
	handler := &capturingHandler{level: slog.LevelInfo}
	logger := slog.New(handler)
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	rc := &RecommendationContext{
		Query:            "playa barata en julio",
		IntentConfidence: 0.85,
		UserID:           "usr_123",
	}

	LogDiscoveryRequest(t.Context(), rc)

	output := handler.String()
	if !handler.hasKeyValue("msg", "discovery request") {
		t.Errorf("expected 'discovery request' log message, got: %s", output)
	}
	if !handler.hasKeyValue("query", "playa barata en julio") {
		t.Errorf("expected query in log, got: %s", output)
	}
}

// =============================================================================
// LogDiscoveryResponse test
// =============================================================================

func TestLogDiscoveryResponse(t *testing.T) {
	handler := &capturingHandler{level: slog.LevelInfo}
	logger := slog.New(handler)
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	rc := &RecommendationContext{
		Query:            "playa en europa",
		IntentConfidence: 0.75,
	}
	candidates := sampleCandidates()

	LogDiscoveryResponse(t.Context(), rc, candidates, "discovery")

	output := handler.String()
	if !handler.hasKeyValue("msg", "discovery response") {
		t.Errorf("expected 'discovery response' log message, got: %s", output)
	}
	if !handler.hasKeyValue("mode", "discovery") {
		t.Errorf("expected mode=discovery in log, got: %s", output)
	}
}

// =============================================================================
// LogDiscoveryClarification test
// =============================================================================

func TestLogDiscoveryClarification(t *testing.T) {
	handler := &capturingHandler{level: slog.LevelInfo}
	logger := slog.New(handler)
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	rc := &RecommendationContext{
		Query:            "quiero viajar",
		IntentConfidence: 0.3,
	}
	question := "¿Qué presupuesto tenés en mente?"

	LogDiscoveryClarification(t.Context(), rc, question)

	output := handler.String()
	if !handler.hasKeyValue("msg", "discovery clarification") {
		t.Errorf("expected 'discovery clarification' log message, got: %s", output)
	}
	if !strings.Contains(output, question) {
		t.Errorf("expected clarification question in log, got: %s", output)
	}
}

// =============================================================================
// LogDiscoveryFallback test
// =============================================================================

func TestLogDiscoveryFallback(t *testing.T) {
	handler := &capturingHandler{level: slog.LevelInfo}
	logger := slog.New(handler)
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	rc := &RecommendationContext{
		Query:            "destino imposible",
		IntentConfidence: 0.2,
	}
	reason := "no_candidates"

	LogDiscoveryFallback(t.Context(), rc, reason)

	output := handler.String()
	if !handler.hasKeyValue("msg", "discovery fallback") {
		t.Errorf("expected 'discovery fallback' log message, got: %s", output)
	}
	if !handler.hasKeyValue("reason", "no_candidates") {
		t.Errorf("expected reason=no_candidates in log, got: %s", output)
	}
}

// =============================================================================
// LogDiscoveryFallback — test con UserID vacío (anónimo)
// =============================================================================

func TestLogDiscoveryFallback_Anonymous(t *testing.T) {
	handler := &capturingHandler{level: slog.LevelInfo}
	logger := slog.New(handler)
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	rc := &RecommendationContext{
		Query:            "viaje",
		IntentConfidence: 0.4,
		// UserID vacío = anónimo
	}
	reason := "discovery_disabled"

	LogDiscoveryFallback(t.Context(), rc, reason)

	output := handler.String()
	if !handler.hasKeyValue("reason", "discovery_disabled") {
		t.Errorf("expected reason in log, got: %s", output)
	}
}

// =============================================================================
// Per-stage metrics tests (WARNING 9)
// =============================================================================

func TestLogCandidatesGenerated(t *testing.T) {
	handler := &capturingHandler{level: slog.LevelInfo}
	logger := slog.New(handler)
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	rc := &RecommendationContext{
		Query:      "playa barata en julio",
		Candidates: sampleCandidates()[:3],
	}
	srcCount := 2

	LogCandidatesGenerated(t.Context(), rc, srcCount)

	output := handler.String()
	if !handler.hasKeyValue("msg", "discovery candidates generated") {
		t.Errorf("expected 'discovery candidates generated' log message, got: %s", output)
	}
	if !handler.hasKeyValue("candidates_count", "3") {
		t.Errorf("expected candidates_count=3 in log, got: %s", output)
	}
	if !handler.hasKeyValue("sources_used", "2") {
		t.Errorf("expected sources_used=2 in log, got: %s", output)
	}
}

func TestLogFormatComplete(t *testing.T) {
	handler := &capturingHandler{level: slog.LevelInfo}
	logger := slog.New(handler)
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	rc := &RecommendationContext{
		Query:                "playa barata en julio",
		RankedCandidates:     sampleCandidates()[:2],
		RequiresClarification: false,
	}

	LogFormatComplete(t.Context(), rc)

	output := handler.String()
	if !handler.hasKeyValue("msg", "discovery format complete") {
		t.Errorf("expected 'discovery format complete' log message, got: %s", output)
	}
	if !handler.hasKeyValue("final_candidates", "2") {
		t.Errorf("expected final_candidates=2 in log, got: %s", output)
	}
	if !handler.hasKeyValue("needs_clarification", "false") {
		t.Errorf("expected needs_clarification=false in log, got: %s", output)
	}
}
