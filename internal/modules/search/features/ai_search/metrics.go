// Observabilidad para el pipeline de discovery.
//
// Funciones de logging estructurado que emiten métricas vía slog.InfoContext
// en cada etapa del pipeline: request, response, clarification, fallback.
package ai_search

import (
	"context"
	"log/slog"
)

// =============================================================================
// LogDiscoveryRequest — registra el inicio de una solicitud de discovery
// =============================================================================

// LogDiscoveryRequest emite un log estructurado al recibir una solicitud de discovery.
// Métricas: discovery_requests_total (implícita vía conteo de logs).
func LogDiscoveryRequest(ctx context.Context, rc *RecommendationContext) {
	slog.InfoContext(ctx, "discovery request",
		slog.String("query", rc.Query),
		slog.Float64("intent_confidence", rc.IntentConfidence),
		slog.String("user_id", rc.UserID),
		slog.String("client_ip", rc.ClientIP),
	)
}

// =============================================================================
// LogDiscoveryResponse — registra la respuesta del pipeline
// =============================================================================

// LogDiscoveryResponse emite un log estructurado al completar el pipeline de discovery.
// Métricas: avg_candidates_generated, candidate_clickthrough_rate (placeholder).
func LogDiscoveryResponse(ctx context.Context, rc *RecommendationContext, candidates []Candidate, mode string) {
	slog.InfoContext(ctx, "discovery response",
		slog.String("query", rc.Query),
		slog.String("mode", mode),
		slog.Int("candidates_count", len(candidates)),
		slog.Float64("intent_confidence", rc.IntentConfidence),
		slog.String("user_id", rc.UserID),
	)
}

// =============================================================================
// LogDiscoveryClarification — registra una pregunta de clarificación
// =============================================================================

// LogDiscoveryClarification emite un log estructurado cuando se genera una
// pregunta de clarificación en vez de recomendaciones.
// Métricas: clarification_rate (needs_clarification / total).
func LogDiscoveryClarification(ctx context.Context, rc *RecommendationContext, question string) {
	slog.InfoContext(ctx, "discovery clarification",
		slog.String("query", rc.Query),
		slog.String("question", question),
		slog.Float64("intent_confidence", rc.IntentConfidence),
		slog.String("user_id", rc.UserID),
		slog.Int("clarification_rounds", rc.ClarificationRounds),
	)
}

// =============================================================================
// LogDiscoveryFallback — registra un fallback (feature flag off, error, etc.)
// =============================================================================

// LogDiscoveryFallback emite un log estructurado cuando el pipeline de discovery
// no puede completarse y se usa un fallback (ej. feature flag desactivado,
// sin candidatos, dataset corrupto).
// Métricas: fallback_rate, empty_candidate_rate.
func LogDiscoveryFallback(ctx context.Context, rc *RecommendationContext, reason string) {
	slog.InfoContext(ctx, "discovery fallback",
		slog.String("query", rc.Query),
		slog.String("reason", reason),
		slog.Float64("intent_confidence", rc.IntentConfidence),
		slog.String("user_id", rc.UserID),
	)
}

// =============================================================================
// Per-stage pipeline metrics (WARNING 9)
// =============================================================================

// LogIntentDetected registra el modo y confianza de la detección de intención.
func LogIntentDetected(ctx context.Context, rc *RecommendationContext) {
	slog.InfoContext(ctx, "discovery intent detected",
		slog.String("query", rc.Query),
		slog.String("mode", string(rc.SearchMode)),
		slog.Float64("confidence", rc.IntentConfidence),
		slog.Bool("needs_clarification", rc.RequiresClarification),
	)
}

// LogCandidatesGenerated registra cuántos candidatos se generaron en la etapa
// de candidate generation del pipeline.
func LogCandidatesGenerated(ctx context.Context, rc *RecommendationContext, srcCount int) {
	slog.InfoContext(ctx, "discovery candidates generated",
		slog.String("query", rc.Query),
		slog.Int("candidates_count", len(rc.Candidates)),
		slog.Int("sources_used", srcCount),
	)
}

// LogFormatComplete registra que la respuesta fue formateada.
func LogFormatComplete(ctx context.Context, rc *RecommendationContext) {
	slog.InfoContext(ctx, "discovery format complete",
		slog.String("query", rc.Query),
		slog.Int("final_candidates", len(rc.RankedCandidates)),
		slog.Bool("needs_clarification", rc.RequiresClarification),
	)
}
