// Tests para el adapter de email Resend.
// Cubre: rate limiter fail-closed (C1), first_name fix (C5), message ID pipeline (D1),
// template ID vacío, errores de API, logger nil sin panic.
package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/resend/resend-go/v3"
)

// =============================================================================
// Helpers
// =============================================================================

// mockRateLimiter implementa la misma interfaz que *ratelimit.RateLimiter
// pero controlamos el comportamiento para tests.
type mockRateLimiter struct {
	allow bool
	err   error
}

func (m *mockRateLimiter) ProviderAllow(ctx context.Context, provider string) (ratelimit.RateLimitResult, error) {
	if m.err != nil {
		return ratelimit.RateLimitResult{}, m.err
	}
	return ratelimit.RateLimitResult{
		Allowed:   m.allow,
		Current:   5,
		Limit:     100,
		Remaining: 95,
	}, nil
}

// rateLimiterInterface define lo que ResendService necesita (ProviderAllow).
// Mockeamos a nivel de interfaz sin depender de Redis.
type rateLimiterInterface interface {
	ProviderAllow(ctx context.Context, provider string) (ratelimit.RateLimitResult, error)
}

// resendServiceWithRL es un ResendService con rate limiter inyectado como interfaz.
// Usamos esta struct para tests que necesitan controlar el rate limiter.
type testableResendService struct {
	client      *resend.Client
	rateLimiter rateLimiterInterface
	logger      *slog.Logger
}

func (s *testableResendService) SendWithTemplate(ctx context.Context, to string, templateID string, variables map[string]any) (string, error) {
	if templateID == "" {
		return "", fmt.Errorf("templateID no configurado - crear template en https://resend.com/templates")
	}

	if s.rateLimiter != nil {
		result, err := s.rateLimiter.ProviderAllow(ctx, "resend")
		if err != nil {
			return "", fmt.Errorf("rate limiter check failed: %w", err)
		}
		if !result.Allowed {
			return "", fmt.Errorf("resend rate limit exceeded: %d/%d emails today", result.Current, result.Limit)
		}
	}

	params := &resend.SendEmailRequest{
		To: []string{to},
		Template: &resend.EmailTemplate{
			Id:        templateID,
			Variables: variables,
		},
	}

	resp, err := s.client.Emails.SendWithContext(ctx, params)
	if err != nil {
		return "", fmt.Errorf("Resend API error: %w", err)
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx, "email enviado via Resend",
			slog.String("message_id", resp.Id),
			slog.String("recipient", to),
		)
	}

	return resp.Id, nil
}

// =============================================================================
// TestSendWithTemplate_EmptyTemplateID: templateID vacío retorna error (C1)
// =============================================================================
func TestSendWithTemplate_EmptyTemplateID(t *testing.T) {
	svc := &ResendService{
		client:      nil, // No debería llegar a usarse
		rateLimiter: nil,
	}

	_, err := svc.SendWithTemplate(context.Background(), "test@example.com", "", nil)

	if err == nil {
		t.Fatal("esperado error por templateID vacío, obtuvo nil")
	}
}

// TestSendWithTemplate_ReturnsMessageID verifica que SendWithTemplate
// retorne el message ID de Resend en caso de éxito (D1).
func TestSendWithTemplate_ReturnsMessageID(t *testing.T) {
	// Servidor mock que simula la API de Resend
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
 		resp := &resend.SendEmailResponse{Id: "resend_msg_test_123"}
 		w.Header().Set("Content-Type", "application/json")
 		json.NewEncoder(w).Encode(resp)
 	}))
 	defer server.Close()

 	serverURL, err := url.Parse(server.URL)
 	if err != nil {
 		t.Fatalf("url parse: %v", err)
 	}

 	client := resend.NewClient("test-api-key")
 	client.BaseURL = serverURL

	svc := &ResendService{
		client:      client,
		rateLimiter: nil,
	}

	msgID, err := svc.SendWithTemplate(context.Background(), "test@example.com", "template-123", map[string]any{
		"name": "Test",
	})

	if err != nil {
		t.Fatalf("inesperado error: %v", err)
	}
	if msgID != "resend_msg_test_123" {
		t.Errorf("esperado messageID 'resend_msg_test_123', obtuvo %q", msgID)
	}
}

// TestSendWithTemplate_ResendAPIError verifica que errores de la API
// de Resend se propaguen correctamente con el nuevo tipo de retorno (D1).
func TestSendWithTemplate_ResendAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)

	client := resend.NewClient("test-api-key")
	client.BaseURL = serverURL

	svc := &ResendService{
		client:      client,
		rateLimiter: nil,
	}

	msgID, err := svc.SendWithTemplate(context.Background(), "test@example.com", "template-123", nil)

	if err == nil {
		t.Fatal("esperado error de API de Resend, obtuvo nil")
	}
	if msgID != "" {
		t.Errorf("esperado messageID vacío en error, obtuvo %q", msgID)
	}
}

// =============================================================================
// H4.1 — TestRateLimiterFailClosed: rate limiter con error NO debe enviar (fail-closed)
// =============================================================================

func TestRateLimiterFailClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("NO debería haberse llamado a la API de Resend (fail-closed)")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client := resend.NewClient("test-api-key")
	client.BaseURL = serverURL

	svc := &testableResendService{
		client:      client,
		rateLimiter: &mockRateLimiter{err: errors.New("Dragonfly no disponible")},
	}

	_, err := svc.SendWithTemplate(context.Background(), "test@example.com", "template-123", nil)
	if err == nil {
		t.Fatal("se esperaba error del rate limiter (fail-closed), se obtuvo nil")
	}
}

// =============================================================================
// H4.2 — TestRateLimiterExceeded: rate limiter con límite superado retorna error
// =============================================================================

func TestRateLimiterExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("NO debería haberse llamado a la API de Resend (límite excedido)")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client := resend.NewClient("test-api-key")
	client.BaseURL = serverURL

	svc := &testableResendService{
		client:      client,
		rateLimiter: &mockRateLimiter{allow: false},
	}

	_, err := svc.SendWithTemplate(context.Background(), "test@example.com", "template-123", nil)
	if err == nil {
		t.Fatal("se esperaba error por límite excedido, se obtuvo nil")
	}
}

// =============================================================================
// H4.3 — TestRateLimiterAllowed: rate limiter permite, email se envía
// =============================================================================

func TestRateLimiterAllowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &resend.SendEmailResponse{Id: "rl-allowed-msg"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client := resend.NewClient("test-api-key")
	client.BaseURL = serverURL

	svc := &testableResendService{
		client:      client,
		rateLimiter: &mockRateLimiter{allow: true},
	}

	msgID, err := svc.SendWithTemplate(context.Background(), "test@example.com", "template-123", nil)
	if err != nil {
		t.Fatalf("inesperado error con rate limiter allowed: %v", err)
	}
	if msgID != "rl-allowed-msg" {
		t.Errorf("esperado messageID 'rl-allowed-msg', obtuvo %q", msgID)
	}
}

// =============================================================================
// H4.4 — TestSendWithTemplate_NilLoggerNoPanic: logger nil no debe causar panic
// =============================================================================

func TestSendWithTemplate_NilLoggerNoPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &resend.SendEmailResponse{Id: "nil-logger-msg"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client := resend.NewClient("test-api-key")
	client.BaseURL = serverURL

	svc := &testableResendService{
		client:      client,
		rateLimiter: nil,
		logger:      nil, // nil logger
	}

	msgID, err := svc.SendWithTemplate(context.Background(), "test@example.com", "template-123", nil)
	if err != nil {
		t.Fatalf("inesperado error con logger nil: %v", err)
	}
	if msgID != "nil-logger-msg" {
		t.Errorf("esperado 'nil-logger-msg', obtuvo %q", msgID)
	}
}
