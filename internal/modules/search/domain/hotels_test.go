package domain

import (
	"encoding/json"
	"testing"
)

// =============================================================================
// JSON Marshal/Unmarshal — HotelSearchResponse
// =============================================================================

func TestHotelSearchResponseJSONRoundtrip(t *testing.T) {
	original := HotelSearchResponse{
		Type:         "hotels",
		ResultsState: "matching",
		Properties: []HotelProperty{
			{
				ID:          "abc123",
				Type:        "hotel",
				Name:        "Grand Hotel",
				Description: "A grand experience",
				BookingURL:  "https://example.com/book",
				GPS: GPS{
					Lat: 40.7128,
					Lng: -74.0060,
				},
				HotelClass:   new(4),
				CheckIn:      "14:00",
				CheckOut:     "11:00",
				Rating: HotelPropertyRating{
					Overall:  new(4.5),
					Location: new(4.2),
				},
				TotalReviews: new(1200),
				Price: HotelPrice{
					Currency: "USD",
					PerNight: PriceDetail{
						Amount:      150.00,
						BeforeTaxes: new(135.50),
					},
					Total: PriceDetail{
						Amount: 450.00,
					},
				},
				Images: []Image{
					{Thumbnail: "https://img.com/thumb.jpg", Original: "https://img.com/orig.jpg"},
				},
				Amenities:    []string{"WiFi", "Pool"},
				NearbyPlaces: []NearbyPlace{},
				FreeCancellation: new(true),
				SpecialOffer:     nil,
				EcoCertified:     new(false),
				ExcludedAmenities: nil,
				Capacity:          nil,
				Ratings: []HotelRatingResponse{
					{Stars: 5, Count: 800},
					{Stars: 4, Count: 300},
				},
				ReviewsBreakdown: []HotelReviewBreakdownResponse{
					{Name: "Service", Description: "Staff service quality", TotalMentioned: 50, Positive: 40, Negative: 5, Neutral: 5},
				},
				Prices: nil,
			},
		},
		Brands: []HotelBrand{
			{
				ID:   1,
				Name: "Mariott",
				Chains: []HotelBrandChain{
					{ID: 10, Name: "ChainOne"},
				},
			},
		},
		Pagination: HotelPagination{
			NextToken: new("abc123"),
			HasMore:   true,
		},
		FromCache: false,
		CachedAt:  nil,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify key fields are present in the JSON output
	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Top-level keys — from_cache/cached_at deben estar presentes con false/null
	for _, key := range []string{"type", "results_state", "properties", "brands", "pagination", "from_cache", "cached_at"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing top-level key %q in JSON output", key)
		}
	}

	// Property fields
	props, ok := generic["properties"].([]interface{})
	if !ok || len(props) == 0 {
		t.Fatal("properties should be a non-empty array")
	}
	prop := props[0].(map[string]interface{})
	for _, key := range []string{"id", "type", "name", "gps", "rating", "price", "images", "amenities"} {
		if _, ok := prop[key]; !ok {
			t.Errorf("missing property key %q in JSON output", key)
		}
	}

	// Verify nil fields are omitted
	if _, ok := prop["capacity"]; ok {
		t.Error("capacity should be omitted when nil")
	}
	if _, ok := prop["prices"]; ok {
		t.Error("prices should be omitted when nil")
	}

	// Roundtrip: unmarshal back and compare
	var restored HotelSearchResponse
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal roundtrip failed: %v", err)
	}

	if restored.Type != original.Type {
		t.Errorf("Type: got %q, want %q", restored.Type, original.Type)
	}
	if len(restored.Properties) != 1 {
		t.Fatalf("Properties count: got %d, want 1", len(restored.Properties))
	}
	rp := restored.Properties[0]
	if rp.ID != "abc123" {
		t.Errorf("Property.ID: got %q, want %q", rp.ID, "abc123")
	}
	if rp.Name != "Grand Hotel" {
		t.Errorf("Property.Name: got %q, want %q", rp.Name, "Grand Hotel")
	}
	if *rp.Rating.Overall != 4.5 {
		t.Errorf("Rating.Overall: got %f, want 4.5", *rp.Rating.Overall)
	}
	if rp.Price.Currency != "USD" {
		t.Errorf("Price.Currency: got %q, want %q", rp.Price.Currency, "USD")
	}
	if rp.Price.PerNight.Amount != 150.00 {
		t.Errorf("Price.PerNight.Amount: got %f, want 150.00", rp.Price.PerNight.Amount)
	}
	if *rp.FreeCancellation != true {
		t.Errorf("FreeCancellation: got %v, want true", *rp.FreeCancellation)
	}
	if rp.ExcludedAmenities != nil {
		t.Error("ExcludedAmenities should be nil for empty slice")
	}

	if len(restored.Brands) != 1 {
		t.Fatalf("Brands count: got %d, want 1", len(restored.Brands))
	}
	if restored.Brands[0].Name != "Mariott" {
		t.Errorf("Brand.Name: got %q, want %q", restored.Brands[0].Name, "Mariott")
	}
	if restored.Pagination.NextToken == nil || *restored.Pagination.NextToken != "abc123" {
		t.Errorf("Pagination.NextToken: got %v, want %q", restored.Pagination.NextToken, "abc123")
	}
}

// =============================================================================
// JSON Marshal/Unmarshal — HotelDetailsResponse
// =============================================================================

func TestHotelDetailsResponseJSONRoundtrip(t *testing.T) {
	addr := "123 Main St, New York, NY 10001"
	dirURL := "https://maps.example.com/directions"

	original := HotelDetailsResponse{
		ID:          "abc123",
		Type:        "hotel",
		Name:        "Grand Hotel",
		Description: "A grand experience",
		BookingURL:  "https://example.com/book",
		GPS: GPS{
			Lat: 40.7128,
			Lng: -74.0060,
		},
		HotelClass: new(4),
		CheckIn:    "14:00",
		CheckOut:   "11:00",
		Rating: HotelPropertyRating{
			Overall:  new(4.5),
			Location: new(4.2),
		},
		TotalReviews: new(1200),
		Price: HotelPrice{
			Currency: "USD",
			PerNight: PriceDetail{Amount: 150.00},
			Total:    PriceDetail{Amount: 450.00},
		},
		Images: []Image{
			{Thumbnail: "https://img.com/thumb.jpg", Original: "https://img.com/orig.jpg"},
		},
		Amenities:    []string{"WiFi", "Pool"},
		NearbyPlaces: []NearbyPlace{},
		Address:      &addr,
		DirectionsURL: &dirURL,
		PriceRange: &HotelPriceRange{
			Currency: "USD",
			Min:      100.00,
			Max:      300.00,
		},
		ExternalReviews: []HotelExternalReview{
			{
				Source:       "TripAdvisor",
				LogoURL:      "https://logo.example.com/ta.png",
				Score:        4.3,
				MaxScore:     5.0,
				TotalReviews: 500,
				FeaturedReview: &HotelFeaturedReview{
					Author:  "John Doe",
					Date:    "2025-01-15",
					Score:   5.0,
					Comment: "Amazing stay!",
					URL:     nil,
				},
			},
		},
		HealthAndSafety: []HotelHealthSafetyCategory{
			{
				Category: "Cleaning",
				Items: []HotelHealthSafetyItem{
					{Name: "Enhanced cleaning", Available: true},
				},
			},
		},
		Sustainability: []HotelSustainabilityCategory{
			{
				Category: "Energy",
				Items: []HotelSustainabilityItem{
					{Name: "Solar panels", Available: true},
				},
			},
		},
		Ratings: []HotelRatingResponse{
			{Stars: 5, Count: 800},
		},
		ReviewsBreakdown: []HotelReviewBreakdownResponse{
			{Name: "Service", Description: "Staff quality", TotalMentioned: 50, Positive: 40, Negative: 5, Neutral: 5},
		},
		FreeCancellation:  new(true),
		SpecialOffer:      nil,
		EcoCertified:      new(false),
		ExcludedAmenities: nil,
		Capacity:          nil,
		FromCache:         false,
		CachedAt:          nil,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Verify detail-specific fields
	for _, key := range []string{"address", "directions_url", "price_range", "external_reviews", "health_and_safety", "sustainability"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing detail key %q in JSON output", key)
		}
	}

	// Verify nil optional fields are omitted
	if _, ok := generic["special_offer"]; ok {
		t.Error("special_offer should be omitted when nil")
	}
	if _, ok := generic["capacity"]; ok {
		t.Error("capacity should be omitted when nil")
	}
	// from_cache/cached_at deben estar presentes con false/null
	if v, ok := generic["from_cache"]; !ok {
		t.Error("from_cache should be present in JSON output")
	} else if v != false {
		t.Errorf("from_cache should be false, got %v", v)
	}
	if _, ok := generic["cached_at"]; !ok {
		t.Error("cached_at should be present in JSON output (as null)")
	}

	// Roundtrip
	var restored HotelDetailsResponse
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal roundtrip failed: %v", err)
	}

	if restored.Type != "hotel" {
		t.Errorf("Type: got %q, want %q", restored.Type, "hotel")
	}
	if restored.Address == nil || *restored.Address != addr {
		t.Errorf("Address: got %v, want %q", restored.Address, addr)
	}
	if restored.PriceRange == nil || restored.PriceRange.Min != 100.00 {
		t.Errorf("PriceRange.Min: got %v, want 100.00", restored.PriceRange)
	}
	if len(restored.ExternalReviews) != 1 {
		t.Fatalf("ExternalReviews count: got %d, want 1", len(restored.ExternalReviews))
	}
	if restored.ExternalReviews[0].FeaturedReview == nil {
		t.Error("FeaturedReview should not be nil")
	}
	if len(restored.HealthAndSafety) != 1 {
		t.Errorf("HealthAndSafety count: got %d, want 1", len(restored.HealthAndSafety))
	}
	if len(restored.Sustainability) != 1 {
		t.Errorf("Sustainability count: got %d, want 1", len(restored.Sustainability))
	}
	if restored.SpecialOffer != nil {
		t.Error("SpecialOffer should be nil after roundtrip")
	}
}

// =============================================================================
// Verify empty/nil slices produce correct JSON
// =============================================================================

func TestHotelPropertyEmptySlicesOmitted(t *testing.T) {
	prop := HotelProperty{
		ID:    "test",
		Type:  "hotel",
		Name:  "Test Hotel",
		GPS:   GPS{Lat: 0, Lng: 0},
		Rating: HotelPropertyRating{Overall: new(3.0)},
		Price: HotelPrice{
			Currency: "USD",
			PerNight: PriceDetail{Amount: 100},
			Total:    PriceDetail{Amount: 200},
		},
		// Leave all slice fields at zero value (nil)
	}

	data, err := json.Marshal(prop)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Fields WITHOUT omitzero tag: nil slices serialize as null (JSON null), not omitted
	for _, key := range []string{"images", "amenities", "nearby_places"} {
		if val, ok := generic[key]; !ok {
			t.Errorf("key %q should be present (no omitzero tag), got missing", key)
		} else if val != nil {
			t.Errorf("key %q should be null for nil slice, got %v", key, val)
		}
	}

	// Fields WITH omitzero tag: nil slices should NOT appear in JSON
	for _, key := range []string{"excluded_amenities", "ratings", "reviews_breakdown", "prices"} {
		if _, ok := generic[key]; ok {
			t.Errorf("key %q should be omitted for nil slice (has omitzero)", key)
		}
	}
}

// =============================================================================
// Verify HotelSearchRequest JSON tags match API contract
// =============================================================================

func TestHotelSearchRequestJSONTags(t *testing.T) {
	req := HotelSearchRequest{
		Query:           "New York",
		CheckInDate:     "2025-06-01",
		CheckOutDate:    "2025-06-05",
		Adults:          2,
		Children:        1,
		ChildrenAges:    []int{5},
		GL:              new("us"),
		HL:              new("en"),
		Currency:        new("USD"),
		MinPrice:        new(50.0),
		MaxPrice:        new(500.0),
		SortBy:          new(3),
		Rating:          new(4),
		PropertyTypes:   []int{1, 2},
		Amenities:       []int{1},
		VacationRentals: true,
		HotelClasses:    []int{3, 4},
		Brands:          []int{100},
		FreeCancellation: true,
		SpecialOffers:    false,
		EcoCertified:     true,
		Bedrooms:         new(2),
		Bathrooms:        new(1),
		PageToken:        new("next-page"),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// All fields should be present
	for _, key := range []string{
		"query", "check_in_date", "check_out_date", "adults", "children",
		"children_ages", "gl", "hl", "currency", "min_price", "max_price",
		"sort_by", "rating", "property_types", "amenities", "vacation_rentals",
		"hotel_classes", "brands", "free_cancellation", "special_offers",
		"eco_certified", "bedrooms", "bathrooms", "page_token",
	} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in HotelSearchRequest JSON", key)
		}
	}

	// Verify page_token value
	if v, ok := generic["page_token"].(string); !ok || v != "next-page" {
		t.Errorf("page_token: got %v, want %q", generic["page_token"], "next-page")
	}
}

// =============================================================================
// Verify HotelDetailsRequest JSON tags
// =============================================================================

func TestHotelDetailsRequestJSONTags(t *testing.T) {
	req := HotelDetailsRequest{
		ID:              "abc123",
		CheckInDate:     "2025-06-01",
		CheckOutDate:    "2025-06-05",
		Adults:          2,
		Children:        0,
		ChildrenAges:    nil,
		GL:              nil,
		HL:              nil,
		Currency:        nil,
		VacationRentals: false,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	for _, key := range []string{"id", "check_in_date", "check_out_date", "adults", "vacation_rentals"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in HotelDetailsRequest JSON", key)
		}
	}

	// nil pointer fields with omitzero should be omitted
	for _, key := range []string{"gl", "hl", "currency"} {
		if _, ok := generic[key]; ok {
			t.Errorf("key %q should be omitted when nil in HotelDetailsRequest JSON", key)
		}
	}

	// nil slices should be omitted
	if _, ok := generic["children_ages"]; ok && generic["children_ages"] != nil {
		t.Error("children_ages should be omitted when nil")
	}
}
