// Tests del handler HTTP para el modo discovery.
// Verifica que las respuestas de discovery se serializan correctamente
// y que los errores se mapean adecuadamente.
package ai_search_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/features/ai_search"
	"github.com/labstack/echo/v5"
)

// =============================================================================
// Handler tests para discovery
// =============================================================================

func newEchoContextForHandler(body string) (*echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/v1/search/ai", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	return c, rec
}

func TestHandler_DiscoveryResponseShape(t *testing.T) {
	// Verifica que una respuesta de discovery tenga los campos esperados
	// en el JSON de salida.
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		DiscoveryEnabled: true,
	})
	// No configuramos AIInterpreter, así que discovery se ejecuta sin LLM

	handler := ai_search.NewHandler(uc, nil, defaultCfg)

	c, rec := newEchoContextForHandler(`{"message": "recomiéndame playa en verano", "search_mode": "discovery"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verificar campos de discovery en la respuesta
	var resp ai_search.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Modo discovery
	if resp.Mode != "discovery" {
		t.Errorf("Mode = %q, want 'discovery'", resp.Mode)
	}
	// FromCache debe ser false
	if resp.FromCache != false {
		t.Errorf("FromCache = %v, want false", resp.FromCache)
	}
	// CachedAt debe ser nil (omitzero)
	// Esto se verifica indirectamente: el campo no debería aparecer en JSON
	if strings.Contains(rec.Body.String(), "cached_at") {
		t.Error("cached_at should be omitted when nil")
	}
}

func TestHandler_DiscoveryClarificationResponse(t *testing.T) {
	// Verifica que una consulta abierta devuelva pregunta de clarificación.
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		DiscoveryEnabled:        true,
		DiscoveryClarifyEnabled: true,
	})
	handler := ai_search.NewHandler(uc, nil, defaultCfg)

	c, rec := newEchoContextForHandler(`{"message": "a dónde puedo viajar", "search_mode": "discovery"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp ai_search.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Debe tener NeedsClarification=true y ClarificationQuestion no vacío
	if !resp.NeedsClarification {
		t.Error("NeedsClarification should be true for open-ended query")
	}
	if resp.ClarificationQuestion == "" {
		t.Error("ClarificationQuestion should not be empty")
	}
}

func TestHandler_NilUseCase_Returns503(t *testing.T) {
	// Nil usecase → 503 Service Unavailable
	handler := ai_search.NewHandler(nil, nil, defaultCfg)

	c, rec := newEchoContextForHandler(`{"message": "viaje"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandler_EmptyMessage_Returns400(t *testing.T) {
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		DiscoveryEnabled: true,
	})
	handler := ai_search.NewHandler(uc, nil, defaultCfg)

	c, _ := newEchoContextForHandler(`{"message": ""}`)

	err := handler.Handle(c)
	if err == nil {
		t.Fatal("expected error for empty message")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if he.Code != http.StatusBadRequest {
		t.Errorf("HTTPError code = %d, want %d", he.Code, http.StatusBadRequest)
	}
}

func TestHandler_CacheControlHeader_Set(t *testing.T) {
	uc := ai_search.NewUseCase(ai_search.UseCaseDeps{
		DiscoveryEnabled: true,
	})
	handler := ai_search.NewHandler(uc, nil, defaultCfg)

	c, rec := newEchoContextForHandler(`{"message": "recomiéndame playa en verano", "search_mode": "discovery"}`)

	err := handler.Handle(c)
	if err != nil {
		t.Fatalf("Handle() unexpected error: %v", err)
	}

	// Cache-Control debe ser "no-store"
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "no-store" {
		t.Errorf("Cache-Control = %q, want 'no-store'", cacheControl)
	}
}
