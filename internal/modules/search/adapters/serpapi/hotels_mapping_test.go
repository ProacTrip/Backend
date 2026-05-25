package serpapi

import (
	"reflect"
	"testing"
)

func TestResolveAmenities_Strings(t *testing.T) {
	result, err := resolveAmenities([]string{"free_wifi", "pool"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 amenities, got %d", len(result))
	}
	if result[0] != 35 {
		t.Errorf("expected free_wifi → 35, got %d", result[0])
	}
	if result[1] != 6 {
		t.Errorf("expected pool → 6, got %d", result[1])
	}
}

func TestResolveAmenities_IntsPassthrough(t *testing.T) {
	result, err := resolveAmenities([]int{1, 3, 7})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !reflect.DeepEqual(result, []int{1, 3, 7}) {
		t.Errorf("expected [1 3 7], got %v", result)
	}
}

func TestResolveAmenities_UnknownString(t *testing.T) {
	_, err := resolveAmenities([]string{"invalid_amenity"})
	if err == nil {
		t.Fatal("expected error for unknown amenity, got nil")
	}
}

func TestResolveAmenities_MixedAny(t *testing.T) {
	result, err := resolveAmenities([]any{"free_wifi", float64(1)})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !reflect.DeepEqual(result, []int{35, 1}) {
		t.Errorf("expected [35 1], got %v", result)
	}
}

func TestResolvePropertyTypes_Strings(t *testing.T) {
	result, err := resolvePropertyTypes([]string{"resort", "hostel"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 property types, got %d", len(result))
	}
	if result[0] != 17 {
		t.Errorf("expected resort → 17, got %d", result[0])
	}
	if result[1] != 14 {
		t.Errorf("expected hostel → 14, got %d", result[1])
	}
}

func TestResolvePropertyTypes_IntsPassthrough(t *testing.T) {
	result, err := resolvePropertyTypes([]int{17, 14})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !reflect.DeepEqual(result, []int{17, 14}) {
		t.Errorf("expected [17 14], got %v", result)
	}
}

func TestResolvePropertyTypes_Unknown(t *testing.T) {
	_, err := resolvePropertyTypes([]string{"invalid_type"})
	if err == nil {
		t.Fatal("expected error for unknown property type, got nil")
	}
}

func TestResolveHotelRating_String(t *testing.T) {
	code, err := resolveHotelRating("4.0_and_up")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if code != 8 {
		t.Errorf("expected 4.0_and_up → 8, got %d", code)
	}
}

func TestResolveHotelRating_IntPassthrough(t *testing.T) {
	code, err := resolveHotelRating(8)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if code != 8 {
		t.Errorf("expected 8, got %d", code)
	}
}

func TestResolveHotelRating_Unknown(t *testing.T) {
	_, err := resolveHotelRating("6.0_and_up")
	if err == nil {
		t.Fatal("expected error for unknown rating, got nil")
	}
}

func TestResolveHotelSortBy_String(t *testing.T) {
	code, err := resolveHotelSortBy("lowest_price")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if code != 5 {
		t.Errorf("expected lowest_price → 5, got %d", code)
	}
}

func TestResolveHotelSortBy_IntPassthrough(t *testing.T) {
	code, err := resolveHotelSortBy(5)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if code != 5 {
		t.Errorf("expected 5, got %d", code)
	}
}

func TestResolveHotelSortBy_Unknown(t *testing.T) {
	_, err := resolveHotelSortBy("invalid_sort")
	if err == nil {
		t.Fatal("expected error for unknown sort, got nil")
	}
}

func TestResolveHotelRating_AllValid(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"3.0_and_up", 6},
		{"3.5_and_up", 7},
		{"4.0_and_up", 8},
		{"4.5_and_up", 9},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			code, err := resolveHotelRating(tt.input)
			if err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
			if code != tt.expected {
				t.Errorf("resolveHotelRating(%q) = %d, want %d", tt.input, code, tt.expected)
			}
		})
	}
}
