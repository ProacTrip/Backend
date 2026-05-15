// Formateador LLM para el pipeline de discovery.
//
// BuildFormattingPrompt construye el prompt para el LLM que formatea las
// recomendaciones en lenguaje natural en español.
//
// Reglas:
//   - Solo describe los candidatos provistos, NO inventes destinos.
//   - Si NeedsClarification: genera pregunta de seguimiento en vez de recomendaciones.
//   - Si no hay candidatos: mensaje honesto de "no encontramos".
package ai_search

import (
	"fmt"
	"strings"
)

// =============================================================================
// BuildFormattingPrompt — construye el prompt para el LLM
// =============================================================================

// BuildFormattingPrompt construye el prompt de sistema + usuario para que el LLM
// formatee las recomendaciones en lenguaje natural en español.
//
// Recibe datos de entorno (language, country, currency) para contextualizar
// la respuesta al usuario.
//
// Si RequiresClarification es true, genera una pregunta de seguimiento en vez
// de listar candidatos. Si no hay candidatos, genera un mensaje honesto.
func BuildFormattingPrompt(rc *RecommendationContext, language, country, currency string) string {
	// Modo clarificación: pregunta de seguimiento, sin candidatos
	if rc.RequiresClarification {
		if rc.ClarificationQuestion != "" {
			return rc.ClarificationQuestion
		}
		return "¿Podrías darme más detalles sobre lo que buscás? Por ejemplo: presupuesto, tipo de destino, o temporada."
	}

	// Sin candidatos: mensaje honesto
	if len(rc.RankedCandidates) == 0 {
		return "No encontramos destinos que coincidan con tus criterios. ¿Querés ajustar algo?"
	}

	// Construir el prompt del sistema
	var sb strings.Builder

	sb.WriteString("Solo describí los destinos provistos. No inventes destinos adicionales.\n\n")
	sb.WriteString("El usuario preguntó: \"" + rc.Query + "\"\n\n")
	sb.WriteString("A continuación los destinos que mejor coinciden:\n\n")

	for i, c := range rc.RankedCandidates {
		sb.WriteString(fmt.Sprintf("%d. %s, %s — ", i+1, c.Destination, c.Country))

		// Budget tier en español
		budgetLabel := budgetTierToSpanish(c.BudgetTier)
		sb.WriteString(fmt.Sprintf("presupuesto %s", budgetLabel))

		// Mejores meses
		if len(c.BestMonths) > 0 {
			sb.WriteString(fmt.Sprintf(" | mejor temporada: %s", formatMonths(c.BestMonths)))
		}

		// Score
		sb.WriteString(fmt.Sprintf(" | score: %.0f%%", c.Score*100))

		// Razones
		if len(c.Reasons) > 0 {
			sb.WriteString(fmt.Sprintf(" | porque: %s", strings.Join(c.Reasons, ", ")))
		}

		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString("Instrucciones:\n")
	sb.WriteString(fmt.Sprintf("- Respondé en %s. El usuario está en %s, usa moneda %s.\n", language, country, currency))
	sb.WriteString("- Seleccioná los 3 a 5 mejores destinos y explicá POR QUÉ en español natural.\n")
	sb.WriteString("- Mencioná: nombre del destino, por qué encaja con la búsqueda, nivel de presupuesto, y mejor temporada.\n")
	sb.WriteString("- NO inventes destinos que no estén en la lista de arriba.\n")
	sb.WriteString("- Sé útil y entusiasta, como un agente de viajes que realmente quiere ayudar.\n")

	return sb.String()
}

// =============================================================================
// Helpers
// =============================================================================

// budgetTierToSpanish traduce el nivel de presupuesto a español.
func budgetTierToSpanish(tier string) string {
	switch tier {
	case "low":
		return "bajo"
	case "medium":
		return "medio"
	case "high":
		return "alto"
	default:
		return tier
	}
}

// formatMonths formatea una lista de meses (1-12) como nombres en español.
func formatMonths(months []int) string {
	if len(months) == 0 {
		return ""
	}
	names := make([]string, len(months))
	for i, m := range months {
		names[i] = monthName(m)
	}
	return strings.Join(names, ", ")
}

// monthName convierte un número de mes (1-12) a su nombre en español.
func monthName(month int) string {
	switch month {
	case 1:
		return "enero"
	case 2:
		return "febrero"
	case 3:
		return "marzo"
	case 4:
		return "abril"
	case 5:
		return "mayo"
	case 6:
		return "junio"
	case 7:
		return "julio"
	case 8:
		return "agosto"
	case 9:
		return "septiembre"
	case 10:
		return "octubre"
	case 11:
		return "noviembre"
	case 12:
		return "diciembre"
	default:
		return ""
	}
}

// =============================================================================
// buildFallbackMessage — mensaje natural sin instrucciones de sistema
// =============================================================================

// buildFallbackMessage genera un mensaje en español natural para el usuario final
// cuando NO hay LLM disponible (degraded mode). A diferencia de BuildFormattingPrompt,
// NO incluye instrucciones del sistema como "Solo describí los destinos provistos".
//
// ISSUE 1: Evita prompt leakage — las instrucciones del LLM no deben aparecer
// en la respuesta al usuario final.
func buildFallbackMessage(rc *RecommendationContext) string {
	// Modo clarificación: pregunta de seguimiento, sin candidatos
	if rc.RequiresClarification {
		if rc.ClarificationQuestion != "" {
			return rc.ClarificationQuestion
		}
		return "¿Podrías darme más detalles sobre lo que buscás? Por ejemplo: presupuesto, tipo de destino, o temporada."
	}

	// Sin candidatos: mensaje honesto
	if len(rc.RankedCandidates) == 0 {
		return "No encontramos destinos que coincidan con tus criterios. ¿Querés ajustar algo?"
	}

	// Construir mensaje natural con los candidatos
	var sb strings.Builder

	// Apertura contextual según la consulta
	query := strings.TrimSpace(rc.Query)
	if query != "" {
		sb.WriteString(fmt.Sprintf("Para tu búsqueda \"%s\", ", query))
	}
	sb.WriteString("podrías considerar ")

	// Listar candidatos (máximo 5)
	maxCandidates := len(rc.RankedCandidates)
	if maxCandidates > 5 {
		maxCandidates = 5
	}

	for i := range maxCandidates {
		c := rc.RankedCandidates[i]
		if i > 0 {
			if i == maxCandidates-1 {
				sb.WriteString(", o ")
			} else {
				sb.WriteString(", ")
			}
		}

		// Destino y país
		if c.Country != "" && c.Country != c.Destination {
			sb.WriteString(fmt.Sprintf("%s (%s)", c.Destination, c.Country))
		} else {
			sb.WriteString(c.Destination)
		}
	}

	sb.WriteString(" — ")

	// Descripción de constraints si hay
	if constraints := rc.ParsedConstraints; constraints.Budget != "" ||
		constraints.Season != "" || len(constraints.TravelStyle) > 0 {
		parts := []string{}
		if len(constraints.TravelStyle) > 0 {
			for _, style := range constraints.TravelStyle {
				switch style {
				case "beach":
					parts = append(parts, "playa")
				case "city":
					parts = append(parts, "ciudad")
				case "nature":
					parts = append(parts, "naturaleza")
				case "culture":
					parts = append(parts, "cultura")
				}
			}
		}
		if constraints.Budget == "low" {
			parts = append(parts, "presupuesto accesible")
		} else if constraints.Budget == "high" {
			parts = append(parts, "presupuesto alto")
		}
		if len(parts) > 0 {
			sb.WriteString("todos destinos con ")
			sb.WriteString(strings.Join(parts, ", "))
			sb.WriteString(".")
		} else {
			sb.WriteString("todos excelentes opciones para tu próximo viaje.")
		}
	} else {
		sb.WriteString("todos excelentes opciones para tu próximo viaje.")
	}

	return sb.String()
}
