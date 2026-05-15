// Estrategia de clarificación para el pipeline de discovery.
//
// Decide si el sistema necesita pedir más información al usuario antes de
// generar recomendaciones, y genera la pregunta de clarificación adecuada.
//
// Reglas de decisión:
//   - Si ClarificationRounds >= 1 → no pedir más clarificación (best-effort).
//   - Si IntentConfidence < 0.5 Y candidates > 10 → necesita clarificación.
//   - Si RequiresClarification desde la detección de intención → necesita clarificación.
//   - Si los constraints están vacíos (sin budget, season, style, region) → necesita clarificación.
//   - Máximo 1 ronda de clarificación.
package ai_search

import "fmt"

// =============================================================================
// Umbrales y constantes
// =============================================================================

const (
	// maxClarificationRounds es el número máximo de rondas de clarificación.
	maxClarificationRounds = 1
	// lowConfidenceThreshold define confianza baja para activar clarificación.
	lowConfidenceThreshold = 0.5
	// manyCandidatesThreshold define cuántos candidatos se consideran "muchos"
	// para activar clarificación por baja confianza.
	manyCandidatesThreshold = 10
	// scoreSpreadThreshold es el spread máximo entre el mejor y peor candidato
	// del top-3 para considerar que hay un ganador claro.
	// Si el spread es menor, las recomendaciones son ambiguas y se pide aclaración.
	scoreSpreadThreshold = 0.15
)

// =============================================================================
// NeedsClarification — decide si se necesita aclaración
// =============================================================================

// NeedsClarification evalúa si el sistema debería pedir más información al usuario
// antes de generar recomendaciones. Retorna true si se necesita clarificación,
// false si se puede proceder con los datos disponibles.
func NeedsClarification(rc *RecommendationContext) bool {
	// Si ya se alcanzó el máximo de rondas, no preguntar más.
	if rc.ClarificationRounds >= maxClarificationRounds {
		return false
	}

	// Regla 1: NeedsClarification desde la detección de intención
	if rc.RequiresClarification {
		return true
	}

	// Regla 2: Baja confianza + muchos candidatos
	if rc.IntentConfidence < lowConfidenceThreshold && len(rc.Candidates) > manyCandidatesThreshold {
		return true
	}

	// Regla 3: Constraints vacíos (sin budget, season, style, region)
	if rc.ParsedConstraints.Budget == "" &&
		rc.ParsedConstraints.Season == "" &&
		len(rc.ParsedConstraints.TravelStyle) == 0 &&
		rc.ParsedConstraints.Region == "" {
		return true
	}

	return false
}

// =============================================================================
// checkScoreSpread — post-ranking score spread check (CRITICAL 4)
// =============================================================================

// checkScoreSpread evalúa si el top-3 de candidatos rankeados tiene un spread
// de scores menor a scoreSpreadThreshold (0.15). Si es así, las recomendaciones
// son ambiguas — ningún candidato sobresale claramente.
//
// Se aplica DESPUÉS del ranking, sobre RankedCandidates, porque los scores
// pre-ranking (popularidad bruta) no son comparables entre sí.
func checkScoreSpread(ranked []Candidate) bool {
	if len(ranked) < 3 {
		return false // necesitamos al menos 3 para calcular spread significativo
	}

	// Encontrar los 3 mejores scores (ya están ordenados por ranking)
	bestScore := ranked[0].Score
	worstScore := bestScore
	count := min(3, len(ranked))
	for i := range count {
		s := ranked[i].Score
		if s > bestScore {
			bestScore = s
		}
		if s < worstScore {
			worstScore = s
		}
	}

	if bestScore <= 0 {
		return false // scores no asignados, no activar
	}

	return (bestScore - worstScore) < scoreSpreadThreshold
}

// =============================================================================
// GenerateClarificationQuestion — genera pregunta de clarificación
// =============================================================================

// GenerateClarificationQuestion genera una pregunta de aclaración en español
// basada en la información que falta en el contexto de recomendación.
// Si todos los campos están presentes, genera una pregunta genérica.
//
// Cuando se detectó el país del usuario (rc.DetectedCountry != ""), la pregunta
// de presupuesto se vuelve contextual: "Estás en {país}. ¿Buscás destinos dentro
// de {país} o preferís viajar al exterior? ¿Tenés un presupuesto aproximado?"
func GenerateClarificationQuestion(rc *RecommendationContext) string {
	c := rc.ParsedConstraints

	// Determinar qué falta y generar pregunta específica
	if c.Budget == "" {
		if rc.DetectedCountry != "" {
			return fmt.Sprintf("Estás en %s. ¿Buscás destinos dentro de %s o preferís viajar al exterior? ¿Tenés un presupuesto aproximado?",
				rc.DetectedCountry, rc.DetectedCountry)
		}
		return "¿Qué presupuesto tenés en mente? ¿Algo económico, medio o te das un gusto?"
	}

	if c.TravelStyle == nil || len(c.TravelStyle) == 0 {
		return "¿Preferís playa, ciudad, montaña o algo más cultural?"
	}

	if c.Season == "" && c.Month == 0 {
		return "¿En qué época del año pensás viajar? ¿Verano, invierno, primavera, otoño?"
	}

	if c.Region == "" {
		return "¿Tenés alguna región en mente? ¿Europa, Asia, América, el Caribe?"
	}

	// Todas las constraints presentes — pregunta genérica
	return "¿Hay algo más que quieras ajustar en tu búsqueda? Podés indicarme presupuesto, temporada o estilo de viaje."
}
