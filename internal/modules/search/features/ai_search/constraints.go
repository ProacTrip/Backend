// Extractor de restricciones de viaje desde consultas en lenguaje natural.
//
// ExtractConstraints analiza la consulta del usuario y extrae restricciones
// estructuradas: presupuesto, temporada, estilo de viaje, región y mes explícito.
// Si no se puede extraer una restricción, el campo queda en su valor cero.
package ai_search

import (
	"sort"
	"strings"
)

// =============================================================================
// Mapas de keywords → valores canónicos
// =============================================================================

// budgetMap mapea keywords en español a niveles de presupuesto.
var budgetMap = map[string]string{
	"barato":    "low",
	"barata":    "low",
	"baratos":   "low",
	"baratas":   "low",
	"económico": "low",
	"económica": "low",
	"lujo":      "high",
	"lujoso":    "high",
	"lujosa":    "high",
	"medio":     "medium",
	"moderado":  "medium",
}

// seasonMap mapea keywords de temporada en español a valores canónicos.
var seasonMap = map[string]string{
	"verano":    "summer",
	"invierno":  "winter",
	"primavera": "spring",
	"otoño":     "fall",
}

// styleMap mapea keywords de estilo de viaje en español a valores canónicos.
var styleMap = map[string]string{
	"playa":    "beach",
	"playas":   "beach",
	"montaña":  "nature",
	"montañas": "nature",
	"naturaleza": "nature",
	"ciudad":   "city",
	"ciudades": "city",
	"urbano":   "city",
	"cultural": "culture",
	"cultura":  "culture",
}

// regionMap mapea keywords de región en español a valores canónicos.
var regionMap = map[string]string{
	"europa":          "europe",
	"asia":            "asia",
	"américa":         "americas",
	"america":         "americas",
	"américa latina":  "americas",
	"america latina":  "americas",
	"latinoamérica":   "americas",
	"latinoamerica":   "americas",
	"sudamérica":      "americas",
	"sudamerica":      "americas",
	"norteamérica":    "americas",
	"norteamerica":    "americas",
	"caribe":          "caribbean",
	"caribeño":        "caribbean",
	"caribeña":        "caribbean",
	"áfrica":          "africa",
	"africa":          "africa",
	"oceanía":         "oceania",
	"oceania":         "oceania",
}

// monthMap mapea nombres de meses en español a números 1-12.
var monthMap = map[string]int{
	"enero":      1,
	"febrero":    2,
	"marzo":      3,
	"abril":      4,
	"mayo":       5,
	"junio":      6,
	"julio":      7,
	"agosto":     8,
	"septiembre": 9,
	"setiembre":  9,
	"octubre":    10,
	"noviembre":  11,
	"diciembre":  12,
}

// =============================================================================
// ExtractConstraints
// =============================================================================

// ExtractConstraints analiza una consulta en lenguaje natural y extrae
// restricciones de viaje estructuradas. Si no se encuentra una restricción,
// el campo correspondiente queda en su valor cero.
// Esta función es no-bloqueante: nunca retorna error.
func ExtractConstraints(query string) Constraints {
	if strings.TrimSpace(query) == "" {
		return Constraints{}
	}

	normalized := strings.ToLower(strings.TrimSpace(query))

	var c Constraints

	// Budget
	for kw, val := range budgetMap {
		if strings.Contains(normalized, kw) {
			c.Budget = val
			break // primera coincidencia gana
		}
	}

	// Season (solo si no hay mes explícito — el mes es más específico)
	hasMonth := false
	for kw, m := range monthMap {
		if strings.Contains(normalized, kw) {
			c.Month = m
			hasMonth = true
			break
		}
	}

	if !hasMonth {
		for kw, val := range seasonMap {
			if strings.Contains(normalized, kw) {
				c.Season = val
				c.ClimateIntent = seasonToClimate(val)
				break
			}
		}
	}

	// Travel style — puede haber múltiples. Preservar orden de aparición en la consulta.
	type styleMatch struct {
		val string
		pos int
	}
	var styleMatches []styleMatch
	for kw, val := range styleMap {
		pos := strings.Index(normalized, kw)
		if pos >= 0 {
			styleMatches = append(styleMatches, styleMatch{val: val, pos: pos})
		}
	}
	// Ordenar por posición de aparición
	sort.Slice(styleMatches, func(i, j int) bool {
		return styleMatches[i].pos < styleMatches[j].pos
	})
	seen := make(map[string]bool)
	for _, sm := range styleMatches {
		if !seen[sm.val] {
			c.TravelStyle = append(c.TravelStyle, sm.val)
			seen[sm.val] = true
		}
	}

	// Region — la más específica gana (frases más largas primero)
	longestRegion := ""
	longestLen := 0
	for kw, val := range regionMap {
		if strings.Contains(normalized, kw) && len(kw) > longestLen {
			longestRegion = val
			longestLen = len(kw)
		}
	}
	c.Region = longestRegion

	return c
}

// =============================================================================
// seasonToClimate — convierte palabra de temporada en intención climática
// =============================================================================

// seasonToClimate mapea una temporada canónica a una intención climática
// multi-hemisferio. Los valores de ClimateIntent son:
//   - "pleasant_warm": quiere calor agradable (ambos hemisferios).
//   - "cool_mild": quiere frío moderado o fresco.
//   - "mild": clima templado (primavera/otoño).
func seasonToClimate(season string) string {
	switch season {
	case "summer":
		return "pleasant_warm"
	case "winter":
		return "cool_mild"
	case "spring", "fall":
		return "mild"
	default:
		return ""
	}
}
