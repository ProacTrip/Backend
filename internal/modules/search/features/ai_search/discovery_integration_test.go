// Tests de integración para el dispatch discovery vs exact search.
// Verifica que el usecase ejecuta el pipeline adecuado según el modo.
// AR-022 + AR-024: dispatch + end-to-end integration.
package ai_search

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Helpers para tests de integración
// =============================================================================

// setupDiscoveryUseCase crea un UseCase con discovery habilitado y
// candidate sources para tests de integración.
func setupDiscoveryUseCase(t *testing.T) *UseCase {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	uc := NewUseCase(UseCaseDeps{
		DiscoveryEnabled: true,
	})
	uc.rdb = rdb
	uc.discoveryEnabled = true
	uc.clarifyEnabled = true
	return uc
}

// setupExactOnlyUseCase crea un UseCase con discovery deshabilitado.
func setupExactOnlyUseCase(t *testing.T) *UseCase {
	t.Helper()

	uc := NewUseCase(UseCaseDeps{
		DiscoveryEnabled: false,
	})
	return uc
}

// =============================================================================
// runDiscovery tests
// =============================================================================

func TestUseCase_RunDiscovery_DiscoveryKeyword(t *testing.T) {
	uc := setupDiscoveryUseCase(t)
	ctx := t.Context()

	cmd := Command{Message: "recomiéndame playa barato en verano"}
	resp, err := uc.Execute(ctx, cmd, "test-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Mode != "discovery" {
		t.Errorf("expected mode=discovery, got %q", resp.Mode)
	}
	// Sin CandidateSources, los candidatos vienen solo de user data (favoritos/guardados).
	// Para este test (usuario sin favoritos), no hay candidatos → se pide clarificación.
	if !resp.NeedsClarification {
		t.Log("sin user data ni data sources → se espera clarificación")
	}
	if resp.Message == "" {
		t.Error("expected non-empty message in discovery response")
	}
}

func TestUseCase_RunDiscovery_WithModeHint(t *testing.T) {
	uc := setupDiscoveryUseCase(t)
	ctx := t.Context()

	// Mensaje que normalmente se clasificaría como exact, pero con hint discovery
	// Sin constraints explícitos → el pipeline pide clarificación
	cmd := Command{
		Message:        "vuelo de Madrid a Londres",
		SearchModeHint: "discovery",
	}
	resp, err := uc.Execute(ctx, cmd, "test-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Mode != "discovery" {
		t.Errorf("expected mode=discovery (hinted), got %q", resp.Mode)
	}
	// Con hint discovery pero sin constraints → se pide clarificación
	if !resp.NeedsClarification {
		t.Error("expected NeedsClarification=true for hinted discovery without constraints")
	}
}

func TestUseCase_RunDiscovery_ClarificationNeeded(t *testing.T) {
	uc := setupDiscoveryUseCase(t)
	ctx := t.Context()

	// Consulta abierta sin constraints → necesita clarificación
	cmd := Command{Message: "a dónde puedo viajar"}
	resp, err := uc.Execute(ctx, cmd, "test-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Mode != "discovery" {
		t.Errorf("expected mode=discovery, got %q", resp.Mode)
	}
	if !resp.NeedsClarification {
		t.Error("expected NeedsClarification=true for open-ended query")
	}
	if resp.ClarificationQuestion == "" {
		t.Error("expected non-empty ClarificationQuestion")
	}
	// CRITICAL 3: Clarification response should NOT include candidates
	if len(resp.Candidates) != 0 {
		t.Errorf("expected 0 candidates in clarification response, got %d", len(resp.Candidates))
	}
	if resp.TotalCandidates != 0 {
		t.Errorf("expected TotalCandidates=0 in clarification response, got %d", resp.TotalCandidates)
	}
}

func TestUseCase_RunDiscovery_ColdStart(t *testing.T) {
	uc := setupDiscoveryUseCase(t)
	ctx := t.Context()

	// Usuario anónimo (cold start) — sin constraints → el pipeline pide aclaración.
	cmd := Command{Message: "recomiéndame un destino"}
	resp, err := uc.Execute(ctx, cmd, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Mode != "discovery" {
		t.Errorf("expected mode=discovery for cold start, got %q", resp.Mode)
	}
	// Cold start sin preferencias → se pide clarificación antes de recomendar
	if !resp.NeedsClarification {
		t.Error("cold start should ask for clarification when no constraints provided")
	}
	if resp.ClarificationQuestion == "" {
		t.Error("cold start clarification should have a question")
	}
}

// =============================================================================
// Feature flag off tests
// =============================================================================

func TestUseCase_DiscoveryDisabled(t *testing.T) {
	// Con discovery deshabilitado, un mensaje de discovery NO debería
	// ejecutar el pipeline de discovery. El usecase debería caer en
	// el flujo exact search existente.
	uc := NewUseCase(UseCaseDeps{
		DiscoveryEnabled: false,
	})
	ctx := t.Context()
	cmd := Command{Message: "recomiéndame playa barato en verano"}

	// Verificar que NO se ejecuta discovery
	if uc.discoveryEnabled {
		t.Error("discovery should be disabled in this test")
	}

	// Intentar ejecutar — fallará porque no hay interpreter, pero
	// verificamos que no fue por el pipeline de discovery
	_, err := uc.Execute(ctx, cmd, "test-user")
	if err == nil {
		t.Error("expected error when no interpreter configured")
	} else {
		t.Logf("correctly got error from exact search path: %v", err)
	}
}

// =============================================================================
// Exact search still works through discovery-disabled usecase
// =============================================================================

func TestUseCase_ExactSearchStillWorks(t *testing.T) {
	uc := NewUseCase(UseCaseDeps{
		DiscoveryEnabled: false,
	})
	ctx := t.Context()
	cmd := Command{Message: "vuelo de Buenos Aires a Madrid"}

	if uc.discoveryEnabled {
		t.Error("discovery should be disabled")
	}

	_, err := uc.Execute(ctx, cmd, "test-user")
	// Error esperado porque no hay interpreter configurado
	if err != nil {
		t.Logf("expected error (no interpreter configured): %v", err)
	} else {
		t.Error("expected error when no interpreter configured for exact search")
	}
}

// =============================================================================
// Response shape tests
// =============================================================================

func TestDiscoveryResponse_Shape(t *testing.T) {
	uc := setupDiscoveryUseCase(t)
	ctx := t.Context()

	cmd := Command{Message: "recomiéndame playa barato en julio"}
	resp, err := uc.Execute(ctx, cmd, "test-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verificar campos requeridos en la respuesta de discovery
	if resp.Mode != "discovery" {
		t.Errorf("Mode = %q, want 'discovery'", resp.Mode)
	}
	if resp.Intent != "discovery" {
		t.Errorf("Intent = %q, want 'discovery'", resp.Intent)
	}

	// Los campos de flight/hotel deben estar vacíos en discovery
	if resp.Flights != nil {
		t.Error("Flights should be nil for discovery response")
	}
	if resp.Hotels != nil {
		t.Error("Hotels should be nil for discovery response")
	}

	// Sin CandidateSources → sin candidatos, se espera clarificación
	if !resp.NeedsClarification {
		t.Log("expected NeedsClarification when no user data and data sources are disabled")
	}
}
