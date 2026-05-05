// airports — IATA airport dataset with embedded JSON and fuzzy resolution.
//
// DESIGN: 3-tier IATA resolution:
//   1. Exact match on IATA code, city name, or aliases (embedded JSON lookup)
//   2. Fuzzy match via sahilm/fuzzy for typo correction
//   3. Returns ErrIATANotFound — caller handles AI fallback (cached in Dragonfly 24h)
//
// The dataset (~300 top airports worldwide) is embedded at compile time via go:embed.
package airports

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/sahilm/fuzzy"
)

// =============================================================================
// Embedded dataset
// =============================================================================

//go:embed iata.json
var datasetFS embed.FS

// =============================================================================
// AirportEntry
// =============================================================================

// AirportEntry represents a single airport in the IATA dataset.
type AirportEntry struct {
	IATA        string   `json:"iata"`
	City        string   `json:"city"`
	Country     string   `json:"country"`
	CountryCode string   `json:"country_code"`
	Aliases     []string `json:"aliases"`
}

// =============================================================================
// Error sentinel
// =============================================================================

// ErrIATANotFound is returned when no airport matches the query.
// The caller (AI search use case) should use the AI fallback to resolve
// the unknown airport name and cache the result in Dragonfly for 24h.
var ErrIATANotFound = errors.New("IATA_NOT_FOUND: no airport found matching the query")

// =============================================================================
// Airport resolution
// =============================================================================

// ResolveIATA resolves an airport query to an AirportEntry.
// Uses a 3-tier fallback strategy:
//  1. Exact match: IATA code, city name, or any alias (case-insensitive, accent-stripped)
//  2. Fuzzy match: typo correction via sahilm/fuzzy
//  3. AI fallback: returns ErrIATANotFound — the caller (AI use case) handles AI resolution
//     and should cache results in Dragonfly with 24h TTL.
//
// The rdb parameter is for future AI-fallback caching; currently unused but
// required by the interface contract. Passing nil is safe.
func ResolveIATA(ctx context.Context, rdb *redis.Client, query string) (*AirportEntry, error) {
	if query == "" {
		return nil, fmt.Errorf("%w: empty query", ErrIATANotFound)
	}

	airports, err := loadDataset()
	if err != nil {
		return nil, fmt.Errorf("load IATA dataset: %w", err)
	}

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty query", ErrIATANotFound)
	}

	// Strip accents BEFORE lowercasing — handles AI-returned queries like
	// "París" (with accent) that should match "Paris" (accents stripped in dataset).
	// IATA codes are always ASCII (no accents) so stripping doesn't affect them.
	normalized := strings.ToLower(StripAccents(trimmed))

	// Tier 1: Exact match (case-insensitive)
	if entry := exactMatch(airports, normalized); entry != nil {
		return entry, nil
	}

	// Tier 2: Fuzzy match (typo correction)
	if entry := fuzzyMatch(airports, normalized); entry != nil {
		return entry, nil
	}

	// Tier 3: AI fallback — caller handles
	_ = rdb // reserved for future AI fallback caching
	_ = ctx // reserved for future AI fallback

	return nil, fmt.Errorf("query %q: %w", query, ErrIATANotFound)
}

// =============================================================================
// Tier 1: Exact match
// =============================================================================

// exactMatch checks for case-insensitive exact match against IATA code,
// city name, and all aliases.
func exactMatch(airports []AirportEntry, normalized string) *AirportEntry {
	for i := range airports {
		a := &airports[i]

		// IATA code match
		if strings.ToLower(a.IATA) == normalized {
			return a
		}

		// City name match
		if strings.ToLower(a.City) == normalized {
			return a
		}

		// Aliases match
		for _, alias := range a.Aliases {
			if strings.ToLower(alias) == normalized {
				return a
			}
		}
	}
	return nil
}

// =============================================================================
// Tier 2: Fuzzy match
// =============================================================================

// fuzzyMatch uses sahilm/fuzzy to find the closest airport match.
// Searches against city name + all aliases as source strings.
// Requires a minimum similarity score to avoid false positives.
// minFuzzyScoreThreshold is the minimum fuzzy match score.
// sahilm/fuzzy returns integer scores; we require at least 15 for
// reasonable similarity on typical query lengths.
const minFuzzyScoreThreshold = 15

// fuzzySource prepares all searchable strings from the dataset.
// Each source maps back to an airport index for lookup after matching.
type fuzzySource struct {
	index int
	text  string
}

func fuzzyMatch(airports []AirportEntry, normalized string) *AirportEntry {
	var sources []string
	sourceMap := make(map[int][]int) // sourceIndex → []airportIndex

	for i := range airports {
		a := &airports[i]

		// Add city name as searchable source
		sourceIdx := len(sources)
		sources = append(sources, strings.ToLower(a.City))
		sourceMap[sourceIdx] = []int{i}

		// Add each alias as searchable source
		for _, alias := range a.Aliases {
			sourceIdx := len(sources)
			sources = append(sources, strings.ToLower(alias))
			sourceMap[sourceIdx] = []int{i}
		}

		// Add IATA code (lowercase) as searchable source for short codes
		sourceIdx = len(sources)
		sources = append(sources, strings.ToLower(a.IATA))
		sourceMap[sourceIdx] = []int{i}
	}

	matches := fuzzy.Find(normalized, sources)
	if len(matches) == 0 {
		return nil
	}

	// The best match must have a score indicating reasonable similarity.
	// sahilm/fuzzy returns integer scores sorted by descending relevance.
	best := matches[0]
	if best.Score < minFuzzyScoreThreshold {
		return nil
	}

	indices := sourceMap[best.Index]
	if len(indices) == 0 {
		return nil
	}

	return &airports[indices[0]]
}

// =============================================================================
// Dataset loading
// =============================================================================

// datasetCache holds the parsed dataset loaded once and reused.
var datasetCache []AirportEntry

// loadDataset loads and parses the embedded iata.json file.
// Results are cached in-memory after first load (dataset is immutable).
func loadDataset() ([]AirportEntry, error) {
	if datasetCache != nil {
		return datasetCache, nil
	}

	data, err := datasetFS.ReadFile("iata.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded iata.json: %w", err)
	}

	var airports []AirportEntry
	if err := json.Unmarshal(data, &airports); err != nil {
		return nil, fmt.Errorf("unmarshal iata.json: %w", err)
	}

	if len(airports) == 0 {
		return nil, errors.New("iata.json is empty: no airports in dataset")
	}

	datasetCache = airports
	return datasetCache, nil
}

// =============================================================================
// Utility — for external consumers that need the full dataset
// =============================================================================

// All returns all airports in the embedded dataset.
// Used when the caller wants full dataset iteration (e.g., validation).
func All() ([]AirportEntry, error) {
	return loadDataset()
}

// =============================================================================
// Accent normalization — handles AI-returned queries with accented chars
// =============================================================================

// StripAccents removes diacritical marks from common European languages.
// Used to normalize AI-extracted city names ("París" → "Paris", "Múnich" → "Munich")
// so they match the accent-free entries in the IATA dataset aliases.
func StripAccents(s string) string {
	replacer := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
		"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U",
		"ü", "u", "Ü", "U", "ñ", "n", "Ñ", "N",
		"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u",
		"À", "A", "È", "E", "Ì", "I", "Ò", "O", "Ù", "U",
		"ä", "a", "ë", "e", "ï", "i", "ö", "o",
		"Ä", "A", "Ë", "E", "Ï", "I", "Ö", "O",
		"â", "a", "ê", "e", "î", "i", "ô", "o", "û", "u",
		"Â", "A", "Ê", "E", "Î", "I", "Ô", "O", "Û", "U",
		"ã", "a", "õ", "o",
		"Ã", "A", "Õ", "O",
	)
	return replacer.Replace(s)
}

// =============================================================================
// AI fallback cache key (reserved for future use when AI resolution is wired)
// =============================================================================

// aiFallbackCacheKey builds the Dragonfly cache key for AI-resolved IATA lookups.
// Reserved for Phase 4 when AIInterpreter is wired for airport name resolution.
//nolint:unused // reserved for Phase 4 implementation
func aiFallbackCacheKey(query string) string {
	return "ai:iata:" + strings.ToLower(strings.TrimSpace(query))
}

// =============================================================================
// Country name → main airport resolution
// =============================================================================
//
// When the AI returns a country name instead of a city (e.g., "Perú" instead of
// "Lima"), the IATA resolver can't find it — country names are not in the
// airport dataset. This map provides country→main-airport fallback so queries
// like "vuelos a Perú" resolve to LIM (Lima).

// countryToMainAirport maps accent-stripped, lowercase country names to the
// IATA code of the main international airport for that country.
var countryToMainAirport = map[string]string{
	// América
	"argentina":    "EZE",
	"bolivia":      "LPB",
	"brasil":       "GRU",
	"chile":        "SCL",
	"colombia":     "BOG",
	"costa rica":   "SJO",
	"cuba":         "HAV",
	"ecuador":      "UIO",
	"el salvador":  "SAL",
	"estados unidos": "JFK",
	"usa":          "JFK",
	"eeuu":         "JFK",
	"guatemala":    "GUA",
	"honduras":     "TGU",
	"mexico":       "MEX",
	"nicaragua":    "MGA",
	"panama":       "PTY",
	"paraguay":     "ASU",
	"peru":         "LIM",
	"puerto rico":  "SJU",
	"republica dominicana": "SDQ",
	"rd":           "SDQ",
	"uruguay":      "MVD",
	"venezuela":    "CCS",
	// Europa
	"alemania":     "FRA",
	"austria":      "VIE",
	"belgica":      "BRU",
	"dinamarca":    "CPH",
	"espana":       "MAD",
	"finlandia":    "HEL",
	"francia":      "CDG",
	"grecia":       "ATH",
	"holanda":      "AMS",
	"paises bajos": "AMS",
	"irlanda":      "DUB",
	"islandia":     "KEF",
	"italia":       "FCO",
	"noruega":      "OSL",
	"polonia":      "WAW",
	"portugal":     "LIS",
	"reino unido":  "LHR",
	"inglaterra":   "LHR",
	"uk":           "LHR",
	"ruisa":        "SVO",
	"suecia":       "ARN",
	"suiza":        "ZRH",
	// Asia / Oceanía
	"australia":    "SYD",
	"china":        "PEK",
	"corea del sur": "ICN",
	"india":        "DEL",
	"indonesia":    "CGK",
	"japon":        "NRT",
	"malasia":      "KUL",
	"nueva zelanda": "AKL",
	"nueva zelandia": "AKL",
	"tailandia":    "BKK",
	// África / Medio Oriente
	"egipto":       "CAI",
	"emiratos arabes": "DXB",
	"dubai":        "DXB",
	"israel":       "TLV",
	"marruecos":    "CMN",
	"qatar":        "DOH",
	"sudafrica":    "JNB",
	"turquia":      "IST",
}

// ResolveCountryToIATA resolves a country name to its main airport IATA code.
// Returns the IATA code and true if found, "" and false otherwise.
// Uses accent-stripped, lowercase matching (same as ResolveIATA).
func ResolveCountryToIATA(countryName string) (string, bool) {
	normalized := strings.ToLower(StripAccents(strings.TrimSpace(countryName)))
	iata, ok := countryToMainAirport[normalized]
	return iata, ok
}

// countryToMainCity maps accent-stripped, lowercase country names to the main
// city string formatted for hotel queries ("City, Country").
var countryToMainCity = map[string]string{
	// América
	"argentina":    "Buenos Aires, Argentina",
	"bolivia":      "La Paz, Bolivia",
	"brasil":       "São Paulo, Brasil",
	"chile":        "Santiago, Chile",
	"colombia":     "Bogotá, Colombia",
	"costa rica":   "San José, Costa Rica",
	"cuba":         "La Habana, Cuba",
	"ecuador":      "Quito, Ecuador",
	"el salvador":  "San Salvador, El Salvador",
	"estados unidos": "Nueva York, Estados Unidos",
	"usa":          "Nueva York, Estados Unidos",
	"eeuu":         "Nueva York, Estados Unidos",
	"guatemala":    "Ciudad de Guatemala, Guatemala",
	"honduras":     "Tegucigalpa, Honduras",
	"mexico":       "Ciudad de México, México",
	"nicaragua":    "Managua, Nicaragua",
	"panama":       "Ciudad de Panamá, Panamá",
	"paraguay":     "Asunción, Paraguay",
	"peru":         "Lima, Perú",
	"puerto rico":  "San Juan, Puerto Rico",
	"republica dominicana": "Santo Domingo, República Dominicana",
	"rd":           "Santo Domingo, República Dominicana",
	"uruguay":      "Montevideo, Uruguay",
	"venezuela":    "Caracas, Venezuela",
	// Europa
	"alemania":     "Berlín, Alemania",
	"austria":      "Viena, Austria",
	"belgica":      "Bruselas, Bélgica",
	"dinamarca":    "Copenhague, Dinamarca",
	"espana":       "Madrid, España",
	"finlandia":    "Helsinki, Finlandia",
	"francia":      "París, Francia",
	"grecia":       "Atenas, Grecia",
	"holanda":      "Ámsterdam, Países Bajos",
	"paises bajos": "Ámsterdam, Países Bajos",
	"irlanda":      "Dublín, Irlanda",
	"islandia":     "Reikiavik, Islandia",
	"italia":       "Roma, Italia",
	"noruega":      "Oslo, Noruega",
	"polonia":      "Varsovia, Polonia",
	"portugal":     "Lisboa, Portugal",
	"reino unido":  "Londres, Reino Unido",
	"inglaterra":   "Londres, Reino Unido",
	"uk":           "Londres, Reino Unido",
	"ruisa":        "Moscú, Rusia",
	"suecia":       "Estocolmo, Suecia",
	"suiza":        "Zúrich, Suiza",
	// Asia / Oceanía
	"australia":    "Sídney, Australia",
	"china":        "Pekín, China",
	"corea del sur": "Seúl, Corea del Sur",
	"india":        "Nueva Delhi, India",
	"indonesia":    "Yakarta, Indonesia",
	"japon":        "Tokio, Japón",
	"malasia":      "Kuala Lumpur, Malasia",
	"nueva zelanda": "Auckland, Nueva Zelanda",
	"nueva zelandia": "Auckland, Nueva Zelanda",
	"tailandia":    "Bangkok, Tailandia",
	// África / Medio Oriente
	"egipto":       "El Cairo, Egipto",
	"emiratos arabes": "Dubái, Emiratos Árabes",
	"dubai":        "Dubái, Emiratos Árabes",
	"israel":       "Tel Aviv, Israel",
	"marruecos":    "Casablanca, Marruecos",
	"qatar":        "Doha, Qatar",
	"sudafrica":    "Johannesburgo, Sudáfrica",
	"turquia":      "Estambul, Turquía",
}

// ResolveCountryToMainCity resolves a country name to the main city string
// formatted for hotel queries ("City, Country").
// Returns the city string and true if found, "" and false otherwise.
func ResolveCountryToMainCity(countryName string) (string, bool) {
	normalized := strings.ToLower(StripAccents(strings.TrimSpace(countryName)))
	city, ok := countryToMainCity[normalized]
	return city, ok
}
