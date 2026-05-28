// Tests para SSE streaming events — search, filters, chunk, done, error.
package ai_search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Task 3.1 — SSE event types (RED: functions don't exist yet)
// =============================================================================

func TestWriteSearchEvent(t *testing.T) {
	w := httptest.NewRecorder()
	searchData := map[string]interface{}{
		"properties": []map[string]interface{}{},
		"results_state": "empty",
	}

	err := WriteSearchEvent(w, "Barcelona, España", "hotels", searchData, nil)
	if err != nil {
		t.Fatalf("WriteSearchEvent failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: search") {
		t.Error("should contain 'event: search'")
	}

	// Parse the SSE data
	lines := strings.Split(strings.TrimSpace(body), "\n")
	var dataLine string
	for i, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
			_ = i
		}
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dataLine), &parsed); err != nil {
		t.Fatalf("unmarshal search event data: %v", err)
	}

	if parsed["destination"] != "Barcelona, España" {
		t.Errorf("destination = %v, want 'Barcelona, España'", parsed["destination"])
	}
	if parsed["type"] != "hotels" {
		t.Errorf("type = %v, want 'hotels'", parsed["type"])
	}
	if _, ok := parsed["data"]; !ok {
		t.Error("should have 'data' field")
	}
}

func TestWriteFiltersEvent(t *testing.T) {
	w := httptest.NewRecorder()
	available := map[string]interface{}{
		"stars":    []int{3, 4, 5},
		"amenities": []string{"wifi", "pool"},
	}
	active := map[string]interface{}{
		"stars": 4,
	}

	err := WriteFiltersEvent(w, available, active)
	if err != nil {
		t.Fatalf("WriteFiltersEvent failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: filters") {
		t.Error("should contain 'event: filters'")
	}

	lines := strings.Split(strings.TrimSpace(body), "\n")
	var dataLine string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dataLine), &parsed); err != nil {
		t.Fatalf("unmarshal filters event data: %v", err)
	}

	avail, ok := parsed["available"].(map[string]interface{})
	if !ok {
		t.Fatal("filters event missing 'available' field")
	}
	if stars, ok := avail["stars"].([]interface{}); !ok || len(stars) != 3 {
		t.Errorf("available.stars should be [3,4,5], got %v", avail["stars"])
	}

	act, ok := parsed["active"].(map[string]interface{})
	if !ok {
		t.Fatal("filters event missing 'active' field")
	}
	if val, ok := act["stars"].(float64); !ok || int(val) != 4 {
		t.Errorf("active.stars should be 4, got %v", act["stars"])
	}
}

func TestWriteChunkEvent(t *testing.T) {
	w := httptest.NewRecorder()
	err := WriteChunkEvent(w, "Hola, ")
	if err != nil {
		t.Fatalf("WriteChunkEvent failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: chunk") {
		t.Error("should contain 'event: chunk'")
	}
	if !strings.Contains(body, "Hola, ") {
		t.Error("should contain chunk content")
	}
}

func TestWriteDoneEvent(t *testing.T) {
	w := httptest.NewRecorder()
	err := WriteDoneEvent(w, "conv_123", 2)
	if err != nil {
		t.Fatalf("WriteDoneEvent failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Error("should contain 'event: done'")
	}

	lines := strings.Split(strings.TrimSpace(body), "\n")
	var dataLine string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dataLine), &parsed); err != nil {
		t.Fatalf("unmarshal done event data: %v", err)
	}

	if parsed["conversation_id"] != "conv_123" {
		t.Errorf("conversation_id = %v, want 'conv_123'", parsed["conversation_id"])
	}
	if parsed["turn_count"] != float64(2) {
		t.Errorf("turn_count = %v, want 2", parsed["turn_count"])
	}
}

func TestWriteErrorEvent(t *testing.T) {
	w := httptest.NewRecorder()
	err := WriteErrorEvent(w, "AI unavailable")
	if err != nil {
		t.Fatalf("WriteErrorEvent failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Error("should contain 'event: error'")
	}
	if !strings.Contains(body, "AI unavailable") {
		t.Error("should contain error message")
	}
}

// =============================================================================
// Task 3.4 — SSE event ordering test
// =============================================================================

func TestSSEEventOrdering(t *testing.T) {
	// Verify the expected SSE event sequence: chunk → search → chunk → filters → done
	w := httptest.NewRecorder()

	WriteChunkEvent(w, "Buscando hoteles...")
	WriteSearchEvent(w, "Madrid", "hotels", map[string]string{"results_state": "complete"}, nil)
	WriteChunkEvent(w, "Encontré 5 hoteles en Madrid.")
	WriteFiltersEvent(w, map[string]interface{}{"stars": []int{3, 4, 5}}, map[string]interface{}{})
	WriteDoneEvent(w, "conv_001", 1)

	body := w.Body.String()

	// Extract event sequence
	lines := strings.Split(body, "\n")
	var events []string
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			events = append(events, strings.TrimPrefix(line, "event: "))
		}
	}

	wantOrder := []string{"chunk", "search", "chunk", "filters", "done"}
	if len(events) != len(wantOrder) {
		t.Fatalf("got %d events, want %d: %v", len(events), len(wantOrder), events)
	}
	for i, want := range wantOrder {
		if events[i] != want {
			t.Errorf("events[%d] = %q, want %q", i, events[i], want)
		}
	}
}

func TestFlusherCheck(t *testing.T) {
	// httptest.ResponseRecorder implements http.Flusher
	var w http.ResponseWriter = httptest.NewRecorder()
	if _, ok := w.(http.Flusher); !ok {
		t.Error("httptest.ResponseRecorder should implement http.Flusher")
	}
}

// =============================================================================
// RED: Task 3.1 — WriteMedicalAlertsEvent
// =============================================================================

func TestWriteMedicalAlertsEvent(t *testing.T) {
	w := httptest.NewRecorder()
	alerts := []domain.MedicalAlert{
		{Level: "warning", Type: "allergy", Message: "Alergia detectada: Maní"},
		{Level: "info", Type: "document", Message: "Pasaporte vence en 30 días"},
	}

	err := WriteMedicalAlertsEvent(w, alerts)
	if err != nil {
		t.Fatalf("WriteMedicalAlertsEvent failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: alert") {
		t.Error("should contain 'event: alert'")
	}

	// Parse the SSE data
	lines := strings.Split(strings.TrimSpace(body), "\n")
	var dataLine string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dataLine), &parsed); err != nil {
		t.Fatalf("unmarshal alert event data: %v", err)
	}

	alertsRaw, ok := parsed["alerts"].([]interface{})
	if !ok {
		t.Fatal("missing 'alerts' array in event data")
	}
	if len(alertsRaw) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alertsRaw))
	}

	firstAlert := alertsRaw[0].(map[string]interface{})
	if firstAlert["level"] != "warning" {
		t.Errorf("alert[0].level = %v, want 'warning'", firstAlert["level"])
	}
	if firstAlert["type"] != "allergy" {
		t.Errorf("alert[0].type = %v, want 'allergy'", firstAlert["type"])
	}
	if firstAlert["message"] != "Alergia detectada: Maní" {
		t.Errorf("alert[0].message = %v", firstAlert["message"])
	}
}

func TestWriteMedicalAlertsEvent_EmptyAlerts(t *testing.T) {
	w := httptest.NewRecorder()
	alerts := []domain.MedicalAlert{}

	err := WriteMedicalAlertsEvent(w, alerts)
	if err != nil {
		t.Fatalf("WriteMedicalAlertsEvent with empty alerts failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: alert") {
		t.Error("should contain 'event: alert' even with empty alerts")
	}
}
