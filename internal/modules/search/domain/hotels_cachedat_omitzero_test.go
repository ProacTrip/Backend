// Test de tipo CachedAt y omitzero en tipos de hoteles.
package domain

import (
	"encoding/json"
	"testing"
	"time"
)

// TestCachedAtTypeIsTime verifica que CachedAt es *time.Time, no *string.
func TestCachedAtTypeIsTime(t *testing.T) {
	ts := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)

	// HotelSearchResponse.CachedAt debe ser *time.Time
	hsr := HotelSearchResponse{
		CachedAt: &ts,
	}
	if hsr.CachedAt == nil {
		t.Fatal("CachedAt should be non-nil")
	}
	if !hsr.CachedAt.Equal(ts) {
		t.Errorf("CachedAt = %v, want %v", hsr.CachedAt, ts)
	}

	// HotelDetailsResponse.CachedAt debe ser *time.Time
	hdr := HotelDetailsResponse{
		CachedAt: &ts,
	}
	if hdr.CachedAt == nil {
		t.Fatal("CachedAt should be non-nil")
	}
	if !hdr.CachedAt.Equal(ts) {
		t.Errorf("CachedAt = %v, want %v", hdr.CachedAt, ts)
	}
}

// TestHotelPriceOmitzero verifica que Price zero-value se omite con omitzero.
func TestHotelPriceOmitzero(t *testing.T) {
	resp := HotelDetailsResponse{
		ID:   "test123",
		Type: "hotel",
		Name: "Test Hotel",
		GPS:  GPS{Lat: 1.0, Lng: 2.0},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Price (HotelPrice struct zero-value) debería omitirse con omitzero
	if _, ok := generic["price"]; ok {
		t.Error("zero-value 'price' should be omitted with omitzero")
	}
}

// TestHotelPriceRangeOmitzero verifica que PriceRange zero-value se omite con omitzero.
func TestHotelPriceRangeOmitzero(t *testing.T) {
	resp := HotelDetailsResponse{
		ID:         "test456",
		Type:       "hotel",
		Name:       "Test Hotel 2",
		GPS:        GPS{Lat: 1.0, Lng: 2.0},
		PriceRange: nil, // nil → omitido
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if _, ok := generic["price_range"]; ok {
		t.Error("nil price_range should be omitted")
	}
}
