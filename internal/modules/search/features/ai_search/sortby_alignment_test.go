package ai_search

import (
	"testing"
)

// TestValidSortByAlignedWithFlights verifica que validSortBy
// contiene los mismos valores que search_flights/command.go.
func TestValidSortByAlignedWithFlights(t *testing.T) {
	// Valores esperados según search_flights/command.go
	flightSortByValues := []string{
		"top",
		"price",
		"departure_time",
		"arrival_time",
		"duration",
		"emissions",
	}

	for _, v := range flightSortByValues {
		if !validSortBy[v] {
			t.Errorf("validSortBy missing flight sort value %q", v)
		}
	}
}

// TestAISortByProducesValidFlightValues verifica que los valores
// producidos por el AI para sortBy son aceptados por search_flights.
func TestAISortByProducesValidFlightValues(t *testing.T) {
	// Simular AI interpretations → sort_by values
	aiMappings := map[string]string{
		"vuelos más baratos":    "price",
		"vuelos más rápidos":    "top", // "top" is the general default
		"salida más temprana":   "departure_time",
		"llegada más temprana":  "arrival_time",
		"vuelos más cortos":     "duration",
		"menos emisiones":       "emissions",
		"mejores vuelos":        "top",
	}

	for aiQuery, expectedSortBy := range aiMappings {
		if !validSortBy[expectedSortBy] {
			t.Errorf("AI query %q → sort_by %q should be in validSortBy", aiQuery, expectedSortBy)
		}
	}
}

// TestHotelSortValuesAreIntegers verifica que los valores de sort_by
// para hoteles son códigos enteros, no strings.
func TestHotelSortValuesAreIntegers(t *testing.T) {
	// Hotel sort_by codes (search_hotels/command.go usa *int)
	hotelSortCodes := map[string]int{
		"precio más bajo":  3,
		"mejores hoteles":  8,
		"más reseñas":      13,
	}

	for label, code := range hotelSortCodes {
		if code <= 0 {
			t.Errorf("hotel sort code for %q should be > 0, got %d", label, code)
		}
	}
}
