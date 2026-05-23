// Adapter de email usando Resend.
// Envía emails usando templates del dashboard de Resend.
package email

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/resend/resend-go/v3"
)

// =============================================================================
// Servicio de Email Resend — adapter para el módulo de notificaciones.
// Usa templates configurados en el dashboard de Resend.
// =============================================================================

// ResendService implementa el envío de emails usando la SDK oficial de Resend.
// El "from" se configura en cada template del dashboard de Resend.
type ResendService struct {
	client      *resend.Client
	rateLimiter *ratelimit.RateLimiter
	logger      *slog.Logger
}

// ResendConfig contiene la configuración para Resend.
type ResendConfig struct {
	APIKey      string
	RateLimiter *ratelimit.RateLimiter
	Logger      *slog.Logger
}

// NewResendService crea un nuevo servicio de email con Resend.
func NewResendService(cfg ResendConfig) *ResendService {
	client := resend.NewClient(cfg.APIKey)
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &ResendService{
		client:      client,
		rateLimiter: cfg.RateLimiter,
		logger:      logger,
	}
}

// =============================================================================
// Constantes de templates de Resend
// =============================================================================

// Constantes de templates configurados en https://resend.com/templates.
// El "from" se define en cada template en el dashboard de Resend.
var (
	// TemplateVerifyEmail es el ID del template de verificación de email en Resend.
	TemplateVerifyEmail = "c58c6953-1bf9-41f1-9d8d-26d5d77b9879"
	// TemplateAccountDisabled es el ID del template de cuenta deshabilitada en Resend.
	TemplateAccountDisabled = "d96a15e5-59e2-4c2a-b561-023287e858c5"
	// TemplateAccountEnabled es el ID del template de cuenta habilitada en Resend.
	TemplateAccountEnabled = "01929326-fe76-40cd-83bd-1cfeff4ed477"
)

// =============================================================================
// Envío con templates
// =============================================================================

// SendWithTemplate envía un email usando un template de Resend.
// Retorna el message ID de Resend en caso de éxito.
// Las variables se reemplazan automáticamente en el template.
// El "from" ya está configurado en cada template del dashboard de Resend.
func (s *ResendService) SendWithTemplate(ctx context.Context, to string, templateID string, variables map[string]any) (string, error) {
	if templateID == "" {
		return "", fmt.Errorf("templateID no configurado - crear template en https://resend.com/templates")
	}

	// Rate limiter fail-closed: si falla la verificación, NO enviar el email.
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

	// Log estructurado con slog en vez de fmt.Printf
	if s.logger != nil {
		s.logger.InfoContext(ctx, "email enviado via Resend",
			slog.String("message_id", resp.Id),
			slog.String("recipient", to),
		)
	}

	return resp.Id, nil
}

// =============================================================================
// Health check
// =============================================================================

// HealthCheck verifica que el adapter de Resend esté correctamente configurado
// y que la API key sea válida. Intenta una llamada liviana a la API de Resend
// para validar autenticación y conectividad.
func (s *ResendService) HealthCheck(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("resend client no inicializado")
	}

	// Intentar listar emails con limit=1 para validar la API key sin costo real.
	// Si la API key es inválida, Resend retorna un error de autenticación.
	_, err := s.client.Emails.ListWithOptions(ctx, &resend.ListOptions{
		Limit: new(1),
	})
	if err != nil {
		return fmt.Errorf("resend health check failed: %w", err)
	}

	return nil
}



