// Detector de intención de búsqueda — clasifica consultas en lenguaje natural.
//
// ClassifyIntent analiza la consulta del usuario y determina el modo de búsqueda:
//   - Discovery: consultas abiertas ("recomiéndame", "a dónde", "vacaciones", etc.)
//   - Assisted: consultas comparativas ("parecido a", "similar a", "tipo")
//   - Exact: consultas concretas con parámetros específicos (vuelos, hoteles)
//
// =============================================================================
// CONFIDENCE: ¿Qué significa el porcentaje de confianza?
// =============================================================================
//
// La confianza (IntentResult.Confidence) es un valor entre 0.0 y 1.0 que
// representa qué tan seguro está el detector de que la consulta pertenece
// al modo detectado. NO es una métrica de precisión de la AI ni de calidad
// de resultados — es puramente una heurística de clasificación de intención.
//
// Cómo se calcula:
//   - Discovery: confianza base 0.35 + bonus por densidad de keywords (max 0.45)
//     + bonus por longitud de consulta (max 0.15). Consultas más largas con más
//     keywords de discovery → mayor confianza. Consultas de 1-2 palabras con
//     keywords genéricas ("viaje") → baja confianza (0.35-0.40).
//   - Assisted: confianza base 0.50 + bonus por densidad de keywords assisted
//     en relación al total de palabras (max 1.0). A más keywords assisted
//     específicas, mayor confianza.
//   - Exact: confianza = 1.0 - (discovery_matches * 0.1), clamped a [0.5, 1.0].
//     Si no hay keywords de discovery → 1.0 (alta confianza). Si hay algunas
//     keywords de discovery pero no suficientes para clasificar como discovery
//     → confianza baja hasta 0.5.
//
// ¿Es relevante para desarrolladores frontend?
//   SÍ, pero con moderación. La confianza es una señal útil para decidir si
//   mostrar resultados con convicción o presentar la respuesta como una
//   sugerencia. Reglas prácticas:
//   - Confianza > 0.70: mostrar resultados normalmente ("Encontré estos vuelos")
//   - Confianza 0.50-0.70: mostrar con tono de sugerencia ("Esto podría interesarte")
//   - Confianza < 0.50: considerar pedir aclaración al usuario
//
// ¿Debería el frontend mostrar la confianza al usuario?
//   NO directamente. La confianza es una señal interna para el frontend
//   decidir cómo presentar resultados, no un dato para el usuario final.
//   Mostrar "Confianza: 65%" al usuario es confuso y no aporta valor.
//   Lo que SÍ puede hacer el frontend es ajustar el tono del mensaje:
//   - Alta confianza: "Acá están los vuelos que encontré"
//   - Baja confianza: "Creo que puede interesarte esto, ¿es lo que buscás?"
package ai_search

import (
	"strings"
	"unicode"
)

// =============================================================================
// Keywords de detección
// =============================================================================

// discoveryKeywords son frases que indican intención de descubrimiento.
// El usuario no sabe a dónde ir, quiere inspiración.
var discoveryKeywords = []string{
	"recomienda", "recomiéndame", "recomienden",
	"sugerime", "sugiéreme", "sugerir",
	"ideas para",
	"a dónde", "adónde",
	"dónde viajar", "donde viajar",
	"escapar", "escapada",
	"vacaciones",
	"playa en",
	"barato en",
	"algún lado", "algun lado",
	"cualquier parte",
	// Palabras de discovery genéricas — el usuario quiere inspiración
	"viaje", "viajar", "viajecito",
}

// assistedKeywords son frases que indican búsqueda asistida por similitud.
// El usuario conoce un destino y quiere algo parecido.
var assistedKeywords = []string{
	"parecido a",
	"similar a",
	"tipo",
}

// openPhrases son frases que indican que la consulta es demasiado abierta
// y necesita aclaración antes de recomendar.
var openPhrases = []string{
	"a dónde puedo", "adónde puedo",
	"a dónde ir", "adónde ir",
	"dónde viajar", "donde viajar",
	"algún lado", "algun lado",
	"cualquier parte",
	"no sé a dónde", "no se a donde",
	"quiero viajar", "quiero ir",
	"dónde me recomiendas", "donde me recomiendas",
	"necesito escapar",
	"escapar de",
}

// =============================================================================
// ClassifyIntent
// =============================================================================

// ClassifyIntent analiza una consulta en lenguaje natural y determina el modo
// de búsqueda (Discovery, Assisted, Exact) junto con la confianza y si necesita
// aclaración adicional.
func ClassifyIntent(query string) IntentResult {
	if strings.TrimSpace(query) == "" {
		return IntentResult{
			Mode:              SearchModeExact,
			Confidence:        0,
			NeedsClarification: false,
		}
	}

	normalized := strings.ToLower(strings.TrimSpace(query))
	words := splitWords(normalized)

	// Contar matches de keywords de discovery
	discMatches := countKeywordMatches(normalized, discoveryKeywords)

	// Contar matches de keywords assisted
	assistMatches := countKeywordMatches(normalized, assistedKeywords)

	// Verificar si la consulta es abierta (necesita aclaración)
	needsClar := containsOpenPhrase(normalized)

	// Determinar modo y confianza
	wordCount := len(words)
	if wordCount == 0 {
		wordCount = 1
	}

	if discMatches > 0 {
		// Confianza basada en keyword matches y longitud de la consulta.
		// Consultas cortas (ej. "viaje") son inherentemente ambiguas → baja confianza.
		// Múltiples keywords + consulta larga → alta confianza.
		keywordScore := min(float64(discMatches)*0.15, 0.45) // max 0.45 por keywords
		lengthScore := min(float64(wordCount)*0.05, 0.15)     // max 0.15 por longitud
		confidence := clamp(0.35 + keywordScore + lengthScore)

		// Consultas muy cortas con keywords de discovery necesitan aclaración adicional
		// aunque no contengan frases abiertas explícitas (ej. "viaje", "viajar").
		if !needsClar && wordCount <= 2 {
			needsClar = true
		}

		return IntentResult{
			Mode:                SearchModeDiscovery,
			Confidence:          confidence,
			NeedsClarification:  needsClar,
		}
	}

	if assistMatches > 0 {
		density := float64(assistMatches) / float64(wordCount)
		confidence := clamp(0.5 + density*2.0) // base 0.5 + bonus por densidad
		return IntentResult{
			Mode:                SearchModeAssisted,
			Confidence:          confidence,
			NeedsClarification:  false,
		}
	}

	// Default: ExactSearch con confianza calculada por densidad de discovery keywords.
	// Fórmula: C = 1.0 - (discMatches * 0.1), clamped a [0.5, 1.0].
	// Cuando no hay discovery keywords → C = 1.0 (alta confianza).
	// Si hay algunas keywords de discovery pero no suficientes → C baja a 0.5.
	exactConf := 1.0 - float64(discMatches)*0.1
	if exactConf < 0.5 {
		exactConf = 0.5
	}
	return IntentResult{
		Mode:                SearchModeExact,
		Confidence:          exactConf,
		NeedsClarification:  false,
	}
}

// =============================================================================
// Helpers
// =============================================================================

// countKeywordMatches cuenta cuántas keywords aparecen en el texto normalizado.
func countKeywordMatches(normalized string, keywords []string) int {
	count := 0
	for _, kw := range keywords {
		if strings.Contains(normalized, kw) {
			count++
		}
	}
	return count
}

// containsOpenPhrase verifica si la consulta contiene frases que indican
// que el usuario no tiene idea de a dónde quiere ir.
func containsOpenPhrase(normalized string) bool {
	for _, phrase := range openPhrases {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

// splitWords separa una cadena en palabras, filtrando signos de puntuación.
func splitWords(s string) []string {
	var words []string
	var current strings.Builder

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

// clamp limita un valor al rango [0, 1].
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}
