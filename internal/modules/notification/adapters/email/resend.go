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
// Resend Email Service - Adapter para notification module
// Usa templates de Resend (dashboard)
// =============================================================================

// ResendService implementa el envío de emails usando la SDK oficial de Resend
// El "from" se configura en cada template del dashboard de Resend
type ResendService struct {
	client      *resend.Client
	rateLimiter *ratelimit.RateLimiter
}

// ResendConfig contiene la configuración para Resend
type ResendConfig struct {
	APIKey      string
	RateLimiter *ratelimit.RateLimiter
}

// NewResendService crea un nuevo servicio de email con Resend
func NewResendService(cfg ResendConfig) *ResendService {
	client := resend.NewClient(cfg.APIKey)
	return &ResendService{
		client:      client,
		rateLimiter: cfg.RateLimiter,
	}
}

// =============================================================================
// Constantes de templates de Resend
// =============================================================================

// Constantes de templates - https://resend.com/templates
// El "from" se define en cada template en el dashboard de Resend
var (
	TemplateWelcome        = "a59105e0-e732-490f-8747-3d2a317e1781" // Tu ID real
	TemplateVerifyEmail    = "c58c6953-1bf9-41f1-9d8d-26d5d77b9879" // Template verificación
	TemplateLogin          = ""                                     // TODO: crear y agregar ID
	TemplateResetPassword  = ""                                     // TODO: crear y agregar ID
	TemplateMFA            = ""                                     // TODO: crear y agregar ID
	TemplateAccountBlocked = ""                                     // TODO: crear y agregar ID
)

// =============================================================================
// Envío con templates
// =============================================================================

// SendWithTemplate envía un email usando un template de Resend
// Las variables se reemplazan automáticamente en el template
// El "from" ya está configurado en cada template del dashboard de Resend
func (s *ResendService) SendWithTemplate(ctx context.Context, to string, templateID string, variables map[string]any) error {
	if templateID == "" {
		return fmt.Errorf("templateID no configurado - crear template en https://resend.com/templates")
	}

	if s.rateLimiter != nil {
		result, err := s.rateLimiter.ProviderAllow(ctx, "resend")
		if err != nil {
			slog.Warn("resend rate limit check failed", "error", err)
		} else if !result.Allowed {
			return fmt.Errorf("resend rate limit exceeded: %d/%d emails today", result.Current, result.Limit)
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
		return fmt.Errorf("Resend API error: %w", err)
	}

	// Log para debugging (en producción, usar structured logging)
	if resp != nil && resp.Id != "" {
		fmt.Printf("[Resend] Email sent to %s, ID: %s\n", to, resp.Id)
	}

	return nil
}

// =============================================================================
// Métodos de conveniencia
// =============================================================================

// SendWelcomeEmail envía email de bienvenida
func (s *ResendService) SendWelcomeEmail(ctx context.Context, to, firstName string) error {
	return s.SendWithTemplate(ctx, to, TemplateWelcome, map[string]any{
		"FIRST_NAME": firstName,
	})
}

// SendVerifyEmail envía email de verificación
// El template usa {{{verification_url}}} y {{{first_name}}}
func (s *ResendService) SendVerifyEmail(ctx context.Context, to, verificationURL string) error {
	return s.SendWithTemplate(ctx, to, TemplateVerifyEmail, map[string]any{
		"first_name":       to, // El template espera first_name, usamos el email como valor
		"verification_url": verificationURL,
	})
}

// SendLoginNotification envía notificación de nuevo login
func (s *ResendService) SendLoginNotification(ctx context.Context, to, deviceInfo, location string) error {
	return s.SendWithTemplate(ctx, to, TemplateLogin, map[string]any{
		"DEVICE_INFO": deviceInfo,
		"LOCATION":    location,
	})
}

// SendPasswordResetEmail envía email para reset de contraseña
func (s *ResendService) SendPasswordResetEmail(ctx context.Context, to, resetURL string) error {
	return s.SendWithTemplate(ctx, to, TemplateResetPassword, map[string]any{
		"RESET_URL": resetURL,
	})
}

// SendMFACode envía código MFA
func (s *ResendService) SendMFACode(ctx context.Context, to, code string) error {
	return s.SendWithTemplate(ctx, to, TemplateMFA, map[string]any{
		"MFA_CODE": code,
	})
}

// SendAccountBlockedEmail envía notificación de cuenta bloqueada
func (s *ResendService) SendAccountBlockedEmail(ctx context.Context, to, reason string) error {
	return s.SendWithTemplate(ctx, to, TemplateAccountBlocked, map[string]any{
		"REASON": reason,
	})
}
