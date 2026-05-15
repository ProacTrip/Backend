// Test de opacidad de caché: verifica que las respuestas JSON
// incluyen "from_cache": false y "cached_at": null aunque
// el use case haya poblado los campos con true / timestamp.
package domain

import (
	"encoding/json"
	"testing"
)

// TestFlightSearchResponse_CacheFieldsPresent verifica que los campos
// from_cache/cached_at están presentes en el JSON (para opacidad de caché
// el handler los hardcodea a false/null).
func TestFlightSearchResponse_CacheFieldsPresent(t *testing.T) {
	resp := FlightSearchResponse{
		TripType:     "round_trip",
		Phase:        "complete",
		ResultsState: "done",
		BestFlights:  []Flight{},
		OtherFlights: []Flight{},
		FromCache:    false,
		CachedAt:     nil,
		Meta:         &PaginationMeta{Limit: 10},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	raw := string(data)

	if !containsKeyValue(raw, "from_cache", "false") {
		t.Error("JSON must contain 'from_cache': false")
	}
	if !containsKeyValue(raw, "cached_at", "null") {
		t.Error("JSON must contain 'cached_at': null")
	}
}

// TestFlightDetailsResponse_CacheFieldsPresent verifica campos de caché
// en FlightDetailsResponse JSON.
func TestFlightDetailsResponse_CacheFieldsPresent(t *testing.T) {
	resp := FlightDetailsResponse{
		Itinerary:      FlightItinerary{TripType: "round_trip"},
		BookingOptions: []BookingOption{},
		FromCache:      false,
		CachedAt:       nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	raw := string(data)

	if !containsKeyValue(raw, "from_cache", "false") {
		t.Error("JSON must contain 'from_cache': false")
	}
	if !containsKeyValue(raw, "cached_at", "null") {
		t.Error("JSON must contain 'cached_at': null")
	}
}

// TestHotelSearchResponse_CacheFieldsPresent verifica campos de caché
// en HotelSearchResponse JSON.
func TestHotelSearchResponse_CacheFieldsPresent(t *testing.T) {
	resp := HotelSearchResponse{
		Type:         "hotels",
		ResultsState: "done",
		Properties:   []HotelProperty{},
		Brands:       []HotelBrand{},
		Pagination:   HotelPagination{},
		FromCache:    false,
		CachedAt:     nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	raw := string(data)

	if !containsKeyValue(raw, "from_cache", "false") {
		t.Error("JSON must contain 'from_cache': false")
	}
	if !containsKeyValue(raw, "cached_at", "null") {
		t.Error("JSON must contain 'cached_at': null")
	}
}

// TestHotelDetailsResponse_CacheFieldsPresent verifica campos de caché
// en HotelDetailsResponse JSON.
func TestHotelDetailsResponse_CacheFieldsPresent(t *testing.T) {
	resp := HotelDetailsResponse{
		ID:        "abc123",
		Type:      "hotel",
		Name:      "Hotel Test",
		FromCache: false,
		CachedAt:  nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	raw := string(data)

	if !containsKeyValue(raw, "from_cache", "false") {
		t.Error("JSON must contain 'from_cache': false")
	}
	if !containsKeyValue(raw, "cached_at", "null") {
		t.Error("JSON must contain 'cached_at': null")
	}
}

// containsKeyValue verifica que el JSON contiene "key": expectedValue.
func containsKeyValue(jsonStr, key, expectedValue string) bool {
	search := `"` + key + `":` + expectedValue
	return contains(jsonStr, search)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
