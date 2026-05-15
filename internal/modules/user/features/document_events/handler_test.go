package document_events_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	doc_events "github.com/ProacTrip/Backend/internal/modules/user/features/document_events"
	"github.com/ProacTrip/Backend/internal/modules/user/domain"
	sharedauth "github.com/ProacTrip/Backend/internal/shared/auth"
)

// =============================================================================
// Mock DocRepo
// =============================================================================

type mockDocRepo struct {
	doc *domain.UserDocument
	err error
}

func (m *mockDocRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.UserDocument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.doc, nil
}

// =============================================================================
// Helper: setup Echo with auth middleware and handler
// =============================================================================

func setupSSETestHandler(t *testing.T, docRepo doc_events.DocRepo, rdb *redis.Client, userID uuid.UUID) *echo.Echo {
	t.Helper()

	handler := doc_events.NewHandler(docRepo, rdb)

	e := echo.New()

	// Middleware que inyecta auth claims (simula el auth middleware de producción)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("user_claims", &sharedauth.AccessClaims{
				UserID:    userID,
				Email:     "test@example.com",
				RoleID:    uuid.Nil,
				Role:      "user",
				SessionID: uuid.Nil,
				JTI:       uuid.Nil,
			})
			return next(c)
		}
	})

	e.GET("/documents/:document_id/events", handler.Handle)

	return e
}

// =============================================================================
// Helper: pre-popula el stream con un evento no-terminal.
// Esto evita que sendHistoricalEvents entre al path bloqueante de miniredis
// (XREAD con Block:0 en stream vacío tiene race condition en miniredis v2.37.0).
// =============================================================================

func seedStreamEvent(t *testing.T, rdb *redis.Client, docID uuid.UUID) {
	t.Helper()
	streamKey := fmt.Sprintf("{events}:doc:events:%s", docID.String())
	ctx := context.Background()
	if err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"event":  "processing",
			"status": "queued",
		},
	}).Err(); err != nil {
		t.Fatalf("failed to seed stream event: %v", err)
	}
}

// =============================================================================
// makeSSERequest envía un GET SSE con timeout de contexto corto.
// El handler emite el evento sintético + eventos históricos ANTES de entrar
// al loop bloqueante. El timeout de 500ms alcanza para verificar headers y body
// sin esperar el XREAD de 25s del loop principal.
// =============================================================================

func makeSSERequest(t *testing.T, e *echo.Echo, docID string) *httptest.ResponseRecorder {
	t.Helper()

	// 500ms es suficiente: los checks de doc, headers y evento sintético son < 5ms
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	t.Cleanup(cancel)

	req := httptest.NewRequest(http.MethodGet, "/documents/"+docID+"/events", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	return rec
}

// =============================================================================
// Test 1 — Content-Type debe ser text/event-stream
// =============================================================================

func TestHandle_ContentTypeIsTextEventStream(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	// Cache key that the handler reads: doc:status:{id}
	cacheKey := "doc:status:" + docID.String()
	cacheValue := `{"document_id":"` + docID.String() + `","status":"queued","file_name":"test.pdf","mime_type":"application/pdf"}`
	mr.Set(cacheKey, cacheValue)

	// Pre-populate the stream so XREAD Block:0 in sendHistoricalEvents doesn't hang
	seedStreamEvent(t, rdb, docID)

	docRepo := &mockDocRepo{
		doc: &domain.UserDocument{
			ID:        docID,
			UserID:    userID,
			OCRStatus: domain.OCRStatusQueued,
		},
	}

	e := setupSSETestHandler(t, docRepo, rdb, userID)
	rec := makeSSERequest(t, e, docID.String())

	// Verificar Content-Type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", contentType, "text/event-stream")
	}

	// Verificar Cache-Control
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", cacheControl, "no-cache")
	}

	// Verificar que el body contiene datos SSE
	body := rec.Body.String()
	if !strings.Contains(body, "data:") {
		t.Error("expected SSE 'data:' field in response body")
	}
	if !strings.Contains(body, "event:") {
		t.Error("expected SSE 'event:' field in response body")
	}
}

// =============================================================================
// Test 2 — El handler lee de la clave {doc}:status:{id} en Dragonfly
// =============================================================================

func TestHandle_ReadsFromDocStatusCacheKey(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	// Valor único para verificar que la respuesta viene del cache
	uniqueMarker := "unique-test-marker-7f3a2d1c"
	cacheKey := "doc:status:" + docID.String()
	cacheValue := `{"document_id":"` + docID.String() + `","status":"validating","message":"` + uniqueMarker + `","mime_type":"image/jpeg"}`
	mr.Set(cacheKey, cacheValue)

	// Pre-populate the stream so XREAD Block:0 in sendHistoricalEvents doesn't hang
	seedStreamEvent(t, rdb, docID)

	docRepo := &mockDocRepo{
		doc: &domain.UserDocument{
			ID:        docID,
			UserID:    userID,
			OCRStatus: domain.OCRStatusQueued,
		},
	}

	e := setupSSETestHandler(t, docRepo, rdb, userID)
	rec := makeSSERequest(t, e, docID.String())

	body := rec.Body.String()

	// El evento sintético (late-connection) debe contener el valor del cache
	if !strings.Contains(body, uniqueMarker) {
		t.Errorf("expected response body to contain cached status marker %q, but it did not.\nBody:\n%s", uniqueMarker, body)
	}
}

// =============================================================================
// Test 3 — Verifica que el handler use exactamente el formato de clave {doc}:status:{id}
// =============================================================================

func TestHandle_UsesCorrectCacheKeyFormat(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	docID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())

	expectedKey := "doc:status:" + docID.String()
	statusValue := `{"document_id":"` + docID.String() + `","status":"completed","message":"done"}`

	// Guardar en la clave esperada — si el handler usa otro formato, no encontrará este valor
	mr.Set(expectedKey, statusValue)

	// Pre-populate the stream so XREAD Block:0 in sendHistoricalEvents doesn't hang
	seedStreamEvent(t, rdb, docID)

	docRepo := &mockDocRepo{
		doc: &domain.UserDocument{
			ID:        docID,
			UserID:    userID,
			OCRStatus: domain.OCRStatusCompleted,
		},
	}

	e := setupSSETestHandler(t, docRepo, rdb, userID)
	rec := makeSSERequest(t, e, docID.String())

	body := rec.Body.String()

	// El evento sintético debe contener "completed" que viene del cache (no del fallback DB)
	if !strings.Contains(body, `"completed"`) {
		t.Errorf("expected response body to contain 'completed' from cache, got:\n%s", body)
	}
}
