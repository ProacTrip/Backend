// airlines — IATA airline dataset with embedded JSON and fuzzy resolution.
//
// DESIGN: 2-tier airline resolution:
//  1. Exact match on IATA code, airline name, or aliases (embedded JSON lookup)
//  2. Fuzzy match via sahilm/fuzzy for typo correction
//
// The dataset (~100 top airlines worldwide) is embedded at compile time via go:embed.
package airlines

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
)

// =============================================================================
// Embedded dataset
// =============================================================================

//go:embed airlines.json
var datasetFS embed.FS

// =============================================================================
// AirlineEntry
// =============================================================================

// AirlineEntry represents a single airline in the dataset.
type AirlineEntry struct {
	IATA    string   `json:"iata"`
	ICAO    string   `json:"icao"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Country string   `json:"country"`
}

// =============================================================================
// Error sentinel
// =============================================================================

// ErrAirlineNotFound is returned when no airline matches the query.
var ErrAirlineNotFound = errors.New("AIRLINE_NOT_FOUND: no airline found matching the query")

// =============================================================================
// Airline resolution
// =============================================================================

// ResolveAirline resolves an airline query to an AirlineEntry.
// Uses a 2-tier fallback strategy:
//  1. Exact match: IATA code, airline name, or any alias (case-insensitive)
//  2. Fuzzy match: typo correction via sahilm/fuzzy (score threshold ≥ 15)
func ResolveAirline(query string) (*AirlineEntry, error) {
	if query == "" {
		return nil, fmt.Errorf("%w: empty query", ErrAirlineNotFound)
	}

	airlines, err := loadDataset()
	if err != nil {
		return nil, fmt.Errorf("load airline dataset: %w", err)
	}

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty query", ErrAirlineNotFound)
	}

	normalized := strings.ToLower(trimmed)

	// Tier 1: Exact match (case-insensitive)
	if entry := exactMatch(airlines, normalized); entry != nil {
		return entry, nil
	}

	// Tier 2: Fuzzy match (typo correction)
	if entry := fuzzyMatch(airlines, normalized); entry != nil {
		return entry, nil
	}

	return nil, fmt.Errorf("query %q: %w", query, ErrAirlineNotFound)
}

// ResolveAirlineToIATA resolves an airline name/code to its IATA code.
// Returns the 2-character IATA code, or an error if not found.
func ResolveAirlineToIATA(query string) (string, error) {
	entry, err := ResolveAirline(query)
	if err != nil {
		return "", err
	}
	return entry.IATA, nil
}

// =============================================================================
// Tier 1: Exact match
// =============================================================================

// exactMatch checks for case-insensitive exact match against IATA code,
// airline name, and all aliases.
func exactMatch(airlines []AirlineEntry, normalized string) *AirlineEntry {
	for i := range airlines {
		a := &airlines[i]

		// IATA code match
		if strings.ToLower(a.IATA) == normalized {
			return a
		}

		// Name match
		if strings.ToLower(a.Name) == normalized {
			return a
		}

		// ICAO code match (for 3-letter codes)
		if strings.ToLower(a.ICAO) == normalized {
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

// minFuzzyScoreThreshold is the minimum fuzzy match score.
// sahilm/fuzzy returns integer scores; we require at least 15 for
// reasonable similarity on typical query lengths.
const minFuzzyScoreThreshold = 15

func fuzzyMatch(airlines []AirlineEntry, normalized string) *AirlineEntry {
	var sources []string
	sourceMap := make(map[int][]int) // sourceIndex → []airlineIndex

	for i := range airlines {
		a := &airlines[i]

		// Add name as searchable source
		sourceIdx := len(sources)
		sources = append(sources, strings.ToLower(a.Name))
		sourceMap[sourceIdx] = []int{i}

		// Add each alias as searchable source
		for _, alias := range a.Aliases {
			sourceIdx := len(sources)
			sources = append(sources, strings.ToLower(alias))
			sourceMap[sourceIdx] = []int{i}
		}

		// Add IATA code as searchable source
		sourceIdx = len(sources)
		sources = append(sources, strings.ToLower(a.IATA))
		sourceMap[sourceIdx] = []int{i}
	}

	matches := fuzzy.Find(normalized, sources)
	if len(matches) == 0 {
		return nil
	}

	// The best match must have a score indicating reasonable similarity.
	best := matches[0]
	if best.Score < minFuzzyScoreThreshold {
		return nil
	}

	indices := sourceMap[best.Index]
	if len(indices) == 0 {
		return nil
	}

	return &airlines[indices[0]]
}

// =============================================================================
// Dataset loading
// =============================================================================

// datasetCache holds the parsed dataset loaded once and reused.
var datasetCache []AirlineEntry

// loadDataset loads and parses the embedded airlines.json file.
// Results are cached in-memory after first load (dataset is immutable).
func loadDataset() ([]AirlineEntry, error) {
	if datasetCache != nil {
		return datasetCache, nil
	}

	data, err := datasetFS.ReadFile("airlines.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded airlines.json: %w", err)
	}

	var airlines []AirlineEntry
	if err := json.Unmarshal(data, &airlines); err != nil {
		return nil, fmt.Errorf("unmarshal airlines.json: %w", err)
	}

	if len(airlines) == 0 {
		return nil, errors.New("airlines.json is empty: no airlines in dataset")
	}

	datasetCache = airlines
	return datasetCache, nil
}

// =============================================================================
// Utility — for external consumers that need the full dataset
// =============================================================================

// All returns all airlines in the embedded dataset.
func All() ([]AirlineEntry, error) {
	return loadDataset()
}
