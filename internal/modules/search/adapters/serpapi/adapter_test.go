// Tests for adapter interface satisfaction and mapping functions.
package serpapi

import (
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Task 2.1 — Adapter satisfies FlightProvider
// =============================================================================

func TestAdapterSatisfiesFlightProvider(t *testing.T) {
	// Compile-time check is in adapter.go:
	//   var _ domain.FlightProvider = (*Adapter)(nil)
	//
	// This runtime test verifies the assignment compiles and runs without panic.
	var a *Adapter
	var p domain.FlightProvider = a
	// In Go, interface holding a typed nil pointer is non-nil —
	// the compile-time assertion is the real guarantee.
	_ = p
}

// =============================================================================
// Task 2.2 — Mapping functions produce correct domain types
// =============================================================================

func TestMapImages(t *testing.T) {
	serpImages := []HotelImage{
		{Thumbnail: "https://img.com/thumb.jpg", OriginalImage: "https://img.com/orig.jpg"},
		{Thumbnail: "https://img.com/thumb2.jpg", OriginalImage: "https://img.com/orig2.jpg"},
	}

	result := MapImages(serpImages)

	if len(result) != 2 {
		t.Fatalf("expected 2 images, got %d", len(result))
	}
	if result[0].Thumbnail != "https://img.com/thumb.jpg" {
		t.Errorf("Thumbnail[0]: got %q, want %q", result[0].Thumbnail, "https://img.com/thumb.jpg")
	}
	if result[0].Original != "https://img.com/orig.jpg" {
		t.Errorf("Original[0]: got %q, want %q", result[0].Original, "https://img.com/orig.jpg")
	}

	// Nil input → nil output
	if imgs := MapImages(nil); imgs != nil {
		t.Error("MapImages(nil) should return nil")
	}
}

func TestMapNearbyPlaces(t *testing.T) {
	serpPlaces := []HotelNearbyPlace{
		{
			Name:        "Central Park",
			Description: new("Beautiful park"),
			Rating:      new(4.5),
			Reviews:     new(1000),
			Category:    new("park"),
			GPSCoordinates: &HotelGPS{
				Latitude:  40.7829,
				Longitude: -73.9654,
			},
			Transportations: []HotelTransportation{
				{Type: "walking", Duration: "5 min"},
			},
		},
	}

	result := MapNearbyPlaces(serpPlaces)

	if len(result) != 1 {
		t.Fatalf("expected 1 place, got %d", len(result))
	}
	p := result[0]
	if p.Name != "Central Park" {
		t.Errorf("Name: got %q, want %q", p.Name, "Central Park")
	}
	if p.Category != "park" {
		t.Errorf("Category: got %q, want %q", p.Category, "park")
	}
	if p.GPS == nil || p.GPS.Lat != 40.7829 || p.GPS.Lng != -73.9654 {
		t.Errorf("GPS: got %+v, want (40.7829, -73.9654)", p.GPS)
	}
	if len(p.Transport) != 1 {
		t.Errorf("Transport: got %d, want 1", len(p.Transport))
	}
	if p.Transport[0].Type != "walking" {
		t.Errorf("Transport[0].Type: got %q, want %q", p.Transport[0].Type, "walking")
	}

	// Nil input → nil output
	if places := MapNearbyPlaces(nil); places != nil {
		t.Error("MapNearbyPlaces(nil) should return nil")
	}
}

func TestMapPriceDetail(t *testing.T) {
	lowest := 150.50
	before := 135.25
	sd := HotelRateDetail{
		ExtractedLowest:          &lowest,
		ExtractedBeforeTaxesFees: &before,
	}

	result := MapPriceDetail(sd)

	if result.Amount != 150.50 {
		t.Errorf("Amount: got %f, want 150.50", result.Amount)
	}
	if result.BeforeTaxes == nil || *result.BeforeTaxes != 135.25 {
		t.Errorf("BeforeTaxes: got %v, want 135.25", result.BeforeTaxes)
	}

	// Empty rate detail
	empty := MapPriceDetail(HotelRateDetail{})
	if empty.Amount != 0 {
		t.Errorf("empty Amount: got %f, want 0", empty.Amount)
	}
	if empty.BeforeTaxes != nil {
		t.Error("empty BeforeTaxes should be nil")
	}
}

func TestMapCapacity(t *testing.T) {
	kvs := []HotelEssentialKV{
		{Key: "unit_type", Value: "Villa completa"},
		{Key: "guests", Value: "4"},
		{Key: "bedrooms", Value: "2"},
		{Key: "bathrooms", Value: "1"},
	}

	result := MapCapacity(kvs)

	if result == nil {
		t.Fatal("expected non-nil Capacity")
	}
	if result.UnitType != "Villa completa" {
		t.Errorf("UnitType: got %q, want %q", result.UnitType, "Villa completa")
	}
	if result.Guests == nil || *result.Guests != 4 {
		t.Errorf("Guests: got %v, want 4", result.Guests)
	}
	if result.Bedrooms == nil || *result.Bedrooms != 2 {
		t.Errorf("Bedrooms: got %v, want 2", result.Bedrooms)
	}
	if result.Bathrooms == nil || *result.Bathrooms != 1 {
		t.Errorf("Bathrooms: got %v, want 1", result.Bathrooms)
	}

	// Empty → nil
	if cap := MapCapacity(nil); cap != nil {
		t.Error("MapCapacity(nil) should return nil")
	}
	if cap := MapCapacity([]HotelEssentialKV{}); cap != nil {
		t.Error("MapCapacity(empty) should return nil")
	}
}

func TestMapRatings(t *testing.T) {
	serpRatings := []HotelRating{
		{Stars: 5, Count: 800},
		{Stars: 4, Count: 200},
	}

	result := MapRatings(serpRatings)

	if len(result) != 2 {
		t.Fatalf("expected 2 ratings, got %d", len(result))
	}
	if result[0].Stars != 5 || result[0].Count != 800 {
		t.Errorf("Rating[0]: got %+v, want {5 800}", result[0])
	}
	if result[1].Stars != 4 || result[1].Count != 200 {
		t.Errorf("Rating[1]: got %+v, want {4 200}", result[1])
	}

	if ratings := MapRatings(nil); ratings != nil {
		t.Error("MapRatings(nil) should return nil")
	}
}

func TestMapReviewsBreakdown(t *testing.T) {
	serpBreakdown := []HotelReviewBreakdown{
		{
			Name:           "Service",
			Description:    "Staff quality",
			TotalMentioned: 50,
			Positive:       40,
			Negative:       5,
			Neutral:        5,
		},
	}

	result := MapReviewsBreakdown(serpBreakdown)

	if len(result) != 1 {
		t.Fatalf("expected 1 breakdown, got %d", len(result))
	}
	b := result[0]
	if b.Name != "Service" {
		t.Errorf("Name: got %q, want %q", b.Name, "Service")
	}
	if b.Positive != 40 {
		t.Errorf("Positive: got %d, want 40", b.Positive)
	}
	if b.Negative != 5 {
		t.Errorf("Negative: got %d, want 5", b.Negative)
	}

	if breakdown := MapReviewsBreakdown(nil); breakdown != nil {
		t.Error("MapReviewsBreakdown(nil) should return nil")
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		hasErr   bool
	}{
		{"4", 4, false},
		{"0", 0, false},
		{"123", 123, false},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			n, err := ParseInt(tc.input)
			if tc.hasErr && err == nil {
				t.Errorf("expected error for %q, got nil", tc.input)
			}
			if !tc.hasErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.input, err)
			}
			if n != tc.expected && !tc.hasErr {
				t.Errorf("ParseInt(%q): got %d, want %d", tc.input, n, tc.expected)
			}
		})
	}
}

// =============================================================================
// Task 2.3 — Adapter satisfies HotelProvider
// =============================================================================

func TestAdapterSatisfiesHotelProvider(t *testing.T) {
	// Compile-time check is in hotels.go:
	//   var _ domain.HotelProvider = (*Adapter)(nil)
	//
	// This runtime test verifies the assignment compiles and runs without panic.
	var a *Adapter
	var p domain.HotelProvider = a
	_ = p
}

// TestAdapterSatisfiesBothInterfaces verifies one adapter implements
// both FlightProvider and HotelProvider simultaneously.
func TestAdapterSatisfiesBothInterfaces(t *testing.T) {
	var a *Adapter

	// Assign to both interfaces — compiles only if methods match
	var _ domain.FlightProvider = a
	var _ domain.HotelProvider = a
}

// =============================================================================
// Task 2.3 — Domain request → SerpAPI params conversion
// =============================================================================

func TestDomainRequestToHotelParams_ALL(t *testing.T) {
	req := domain.HotelSearchRequest{
		Query:            "New York",
		CheckInDate:      "2026-06-01",
		CheckOutDate:     "2026-06-05",
		Adults:           2,
		Children:         1,
		ChildrenAges:     []int{3},
		GL:               new("us"),
		HL:               new("en"),
		Currency:         new("USD"),
		MinPrice:         new(50.0),
		MaxPrice:         new(500.0),
		SortBy:           new(3),
		Rating:           new(4),
		PropertyTypes:    []int{1, 2},
		Amenities:        []int{1},
		VacationRentals:  true,
		HotelClasses:     []int{3, 4},
		Brands:           []int{100},
		FreeCancellation: true,
		SpecialOffers:    false,
		EcoCertified:     true,
		Bedrooms:         new(2),
		Bathrooms:        new(1),
		PageToken:        "next-page",
	}

	params := domainRequestToHotelParams(req)

	if params.Query != "New York" {
		t.Errorf("Query: got %q, want %q", params.Query, "New York")
	}
	if params.GL != "us" {
		t.Errorf("GL: got %q, want %q", params.GL, "us")
	}
	if params.HL != "en" {
		t.Errorf("HL: got %q, want %q", params.HL, "en")
	}
	if params.Currency != "USD" {
		t.Errorf("Currency: got %q, want %q", params.Currency, "USD")
	}
	if params.CheckInDate != "2026-06-01" {
		t.Errorf("CheckInDate: got %q, want %q", params.CheckInDate, "2026-06-01")
	}
	if params.CheckOutDate != "2026-06-05" {
		t.Errorf("CheckOutDate: got %q, want %q", params.CheckOutDate, "2026-06-05")
	}
	if params.Adults != 2 {
		t.Errorf("Adults: got %d, want 2", params.Adults)
	}
	if params.Children != 1 {
		t.Errorf("Children: got %d, want 1", params.Children)
	}
	if len(params.ChildrenAges) != 1 || params.ChildrenAges[0] != 3 {
		t.Errorf("ChildrenAges: got %v, want [3]", params.ChildrenAges)
	}
	if params.MinPrice == nil || *params.MinPrice != 50.0 {
		t.Errorf("MinPrice: got %v, want 50.0", params.MinPrice)
	}
	if params.MaxPrice == nil || *params.MaxPrice != 500.0 {
		t.Errorf("MaxPrice: got %v, want 500.0", params.MaxPrice)
	}
	if params.SortBy == nil || *params.SortBy != 3 {
		t.Errorf("SortBy: got %v, want 3", params.SortBy)
	}
	if params.Rating == nil || *params.Rating != 4 {
		t.Errorf("Rating: got %v, want 4", params.Rating)
	}
	if len(params.PropertyTypes) != 2 {
		t.Errorf("PropertyTypes: got %v, want [1,2]", params.PropertyTypes)
	}
	if len(params.Amenities) != 1 {
		t.Errorf("Amenities: got %v, want [1]", params.Amenities)
	}
	if params.VacationRentals != true {
		t.Error("VacationRentals should be true")
	}
	if len(params.HotelClasses) != 2 {
		t.Errorf("HotelClasses: got %v, want [3,4]", params.HotelClasses)
	}
	if len(params.Brands) != 1 {
		t.Errorf("Brands: got %v, want [100]", params.Brands)
	}
	if params.FreeCancellation != true {
		t.Error("FreeCancellation should be true")
	}
	if params.SpecialOffers != false {
		t.Error("SpecialOffers should be false")
	}
	if params.EcoCertified != true {
		t.Error("EcoCertified should be true")
	}
	if params.Bedrooms == nil || *params.Bedrooms != 2 {
		t.Errorf("Bedrooms: got %v, want 2", params.Bedrooms)
	}
	if params.Bathrooms == nil || *params.Bathrooms != 1 {
		t.Errorf("Bathrooms: got %v, want 1", params.Bathrooms)
	}
	if params.PageToken != "next-page" {
		t.Errorf("PageToken: got %q, want %q", params.PageToken, "next-page")
	}
}

func TestDomainRequestToHotelParams_NilOptionals(t *testing.T) {
	req := domain.HotelSearchRequest{
		Query:        "Barcelona",
		CheckInDate:  "2026-07-01",
		CheckOutDate: "2026-07-05",
		Adults:       1,
		// All optional fields left at zero/nil
	}

	params := domainRequestToHotelParams(req)

	if params.Query != "Barcelona" {
		t.Errorf("Query: got %q", params.Query)
	}
	if params.GL != "" {
		t.Errorf("GL should be empty when nil, got %q", params.GL)
	}
	if params.HL != "" {
		t.Errorf("HL should be empty when nil, got %q", params.HL)
	}
	if params.Currency != "" {
		t.Errorf("Currency should be empty when nil, got %q", params.Currency)
	}
	if params.MinPrice != nil {
		t.Error("MinPrice should be nil")
	}
	if params.MaxPrice != nil {
		t.Error("MaxPrice should be nil")
	}
	if params.SortBy != nil {
		t.Error("SortBy should be nil")
	}
	if params.Rating != nil {
		t.Error("Rating should be nil")
	}
	if params.VacationRentals != false {
		t.Error("VacationRentals should default to false")
	}
	if params.FreeCancellation != false {
		t.Error("FreeCancellation should default to false")
	}
}

func TestDomainRequestToDetailsParams(t *testing.T) {
	req := domain.HotelDetailsRequest{
		ID:              "prop-123",
		CheckInDate:     "2026-08-01",
		CheckOutDate:    "2026-08-05",
		Adults:          2,
		Children:        1,
		ChildrenAges:    []int{5, 3},
		GL:              new("es"),
		HL:              new("es"),
		Currency:        new("EUR"),
		VacationRentals: true,
	}

	params := domainRequestToDetailsParams(req)

	if params.PropertyToken != "prop-123" {
		t.Errorf("PropertyToken: got %q, want %q", params.PropertyToken, "prop-123")
	}
	if params.CheckInDate != "2026-08-01" {
		t.Errorf("CheckInDate: got %q", params.CheckInDate)
	}
	if params.CheckOutDate != "2026-08-05" {
		t.Errorf("CheckOutDate: got %q", params.CheckOutDate)
	}
	if params.Adults != 2 {
		t.Errorf("Adults: got %d", params.Adults)
	}
	if params.Children != 1 {
		t.Errorf("Children: got %d", params.Children)
	}
	if len(params.ChildrenAges) != 2 {
		t.Errorf("ChildrenAges: got %v, want [5,3]", params.ChildrenAges)
	}
	if params.GL != "es" {
		t.Errorf("GL: got %q", params.GL)
	}
	if params.HL != "es" {
		t.Errorf("HL: got %q", params.HL)
	}
	if params.Currency != "EUR" {
		t.Errorf("Currency: got %q", params.Currency)
	}
	if params.VacationRentals != true {
		t.Error("VacationRentals should be true")
	}
}

func TestPtrStrDomain(t *testing.T) {
	if s := ptrStrDomain(nil); s != "" {
		t.Errorf("ptrStrDomain(nil): got %q, want empty", s)
	}
	val := "hello"
	if s := ptrStrDomain(&val); s != "hello" {
		t.Errorf("ptrStrDomain(&val): got %q, want %q", s, "hello")
	}
}

// =============================================================================
// Task 2.3 — Hotel mapping functions (serpapi DTO → domain types)
// =============================================================================

func TestMapSingleHotelProperty_Hotel(t *testing.T) {
	sp := HotelProperty{
		PropertyToken:      "token-abc",
		Type:               "hotel",
		Name:               "Grand Hotel",
		Description:        "A luxury hotel",
		Link:               "https://example.com/hotel",
		GPSCoordinates:     HotelGPS{Latitude: 40.7128, Longitude: -74.0060},
		ExtractedHotelClass: new(5),
		CheckInTime:        "14:00",
		CheckOutTime:       "11:00",
		OverallRating:      new(4.5),
		LocationRating:     new(4.2),
		Reviews:            new(200),
		RatePerNight: HotelRateDetail{
			ExtractedLowest:          new(150.0),
			ExtractedBeforeTaxesFees: new(135.0),
		},
		TotalRate: HotelRateDetail{
			ExtractedLowest: new(450.0),
		},
		Images: []HotelImage{
			{Thumbnail: "thumb.jpg", OriginalImage: "orig.jpg"},
		},
		Amenities:     []string{"pool", "gym"},
		NearbyPlaces:  nil,
		Ratings:       nil,
		ReviewsBreakdown: nil,
		FreeCancellation: new(true),
		SpecialOffer:     new(false),
		EcoCertified:     new(true),
	}

	p := mapSingleHotelProperty(sp, "USD")

	if p.ID != "token-abc" {
		t.Errorf("ID: got %q, want %q", p.ID, "token-abc")
	}
	if p.Type != "hotel" {
		t.Errorf("Type: got %q, want %q", p.Type, "hotel")
	}
	if p.Name != "Grand Hotel" {
		t.Errorf("Name: got %q, want %q", p.Name, "Grand Hotel")
	}
	if p.Description != "A luxury hotel" {
		t.Errorf("Description: got %q", p.Description)
	}
	if p.BookingURL != "https://example.com/hotel" {
		t.Errorf("BookingURL: got %q", p.BookingURL)
	}
	if p.GPS.Lat != 40.7128 || p.GPS.Lng != -74.0060 {
		t.Errorf("GPS: got (%.4f, %.4f), want (40.7128, -74.0060)", p.GPS.Lat, p.GPS.Lng)
	}
	if p.HotelClass == nil || *p.HotelClass != 5 {
		t.Errorf("HotelClass: got %v, want 5", p.HotelClass)
	}
	if p.CheckIn != "14:00" || p.CheckOut != "11:00" {
		t.Errorf("CheckIn/Out: got %q/%q, want 14:00/11:00", p.CheckIn, p.CheckOut)
	}
	if p.Rating.Overall == nil || *p.Rating.Overall != 4.5 {
		t.Errorf("Overall: got %v, want 4.5", p.Rating.Overall)
	}
	if p.Rating.Location == nil || *p.Rating.Location != 4.2 {
		t.Errorf("Location: got %v, want 4.2", p.Rating.Location)
	}
	if p.TotalReviews == nil || *p.TotalReviews != 200 {
		t.Errorf("TotalReviews: got %v, want 200", p.TotalReviews)
	}
	if p.Price.Currency != "USD" {
		t.Errorf("Currency: got %q, want USD", p.Price.Currency)
	}
	if p.Price.PerNight.Amount != 150.0 {
		t.Errorf("PerNight.Amount: got %f, want 150.0", p.Price.PerNight.Amount)
	}
	if p.Price.PerNight.BeforeTaxes == nil || *p.Price.PerNight.BeforeTaxes != 135.0 {
		t.Errorf("PerNight.BeforeTaxes: got %v, want 135.0", p.Price.PerNight.BeforeTaxes)
	}
	if len(p.Images) != 1 || p.Images[0].Thumbnail != "thumb.jpg" {
		t.Errorf("Images: got %d images", len(p.Images))
	}
	if len(p.Amenities) != 2 || p.Amenities[0] != "pool" {
		t.Errorf("Amenities: got %v", p.Amenities)
	}
	if p.FreeCancellation == nil || *p.FreeCancellation != true {
		t.Errorf("FreeCancellation: got %v, want true", p.FreeCancellation)
	}
	if p.SpecialOffer == nil || *p.SpecialOffer != false {
		t.Errorf("SpecialOffer: got %v, want false", p.SpecialOffer)
	}
	if p.EcoCertified == nil || *p.EcoCertified != true {
		t.Errorf("EcoCertified: got %v, want true", p.EcoCertified)
	}
	// Hotel (non-VR) should NOT have VR-specific fields
	if len(p.ExcludedAmenities) != 0 {
		t.Errorf("Hotel should have empty ExcludedAmenities, got %v", p.ExcludedAmenities)
	}
	if p.Capacity != nil {
		t.Errorf("Hotel should have nil Capacity, got %+v", p.Capacity)
	}
}

func TestMapSingleHotelProperty_VR(t *testing.T) {
	sp := HotelProperty{
		PropertyToken:  "vr-token",
		Type:           "vacation_rental",
		Name:           "Beach Villa",
		GPSCoordinates: HotelGPS{Latitude: 36.0, Longitude: 28.0},
		RatePerNight: HotelRateDetail{
			ExtractedLowest: new(200.0),
		},
		TotalRate: HotelRateDetail{
			ExtractedLowest: new(600.0),
		},
		ExcludedAmenities: []string{"air_conditioning"},
		EssentialInfo: EssentialInfoField{
			{Key: "unit_type", Value: "Villa completa"},
			{Key: "guests", Value: "6"},
			{Key: "bedrooms", Value: "3"},
			{Key: "bathrooms", Value: "2"},
		},
	}

	p := mapSingleHotelProperty(sp, "EUR")

	if p.Type != "vacation_rental" {
		t.Errorf("Type: got %q, want vacation_rental", p.Type)
	}
	if len(p.ExcludedAmenities) != 1 || p.ExcludedAmenities[0] != "air_conditioning" {
		t.Errorf("ExcludedAmenities: got %v", p.ExcludedAmenities)
	}
	if p.Capacity == nil {
		t.Fatal("VR should have Capacity")
	}
	if p.Capacity.UnitType != "Villa completa" {
		t.Errorf("Capacity.UnitType: got %q", p.Capacity.UnitType)
	}
	if p.Capacity.Guests == nil || *p.Capacity.Guests != 6 {
		t.Errorf("Capacity.Guests: got %v, want 6", p.Capacity.Guests)
	}
	if p.Capacity.Bedrooms == nil || *p.Capacity.Bedrooms != 3 {
		t.Errorf("Capacity.Bedrooms: got %v", p.Capacity.Bedrooms)
	}
	if p.Capacity.Bathrooms == nil || *p.Capacity.Bathrooms != 2 {
		t.Errorf("Capacity.Bathrooms: got %v", p.Capacity.Bathrooms)
	}
}

func TestMapHotelProperties(t *testing.T) {
	// Nil input → nil output
	if props := mapHotelProperties(nil, "USD"); props != nil {
		t.Error("mapHotelProperties(nil) should return nil")
	}

	// Empty input → empty output (non-nil slice, len=0)
	if props := mapHotelProperties([]HotelProperty{}, "USD"); len(props) != 0 {
		t.Errorf("mapHotelProperties([]) should return empty slice, got len=%d", len(props))
	}

	// Two properties
	sp := []HotelProperty{
		{
			PropertyToken:   "p1",
			Type:            "hotel",
			Name:            "Hotel One",
			GPSCoordinates:  HotelGPS{Latitude: 1.0, Longitude: 2.0},
			RatePerNight:    HotelRateDetail{ExtractedLowest: new(100.0)},
		},
		{
			PropertyToken:   "p2",
			Type:            "hotel",
			Name:            "Hotel Two",
			GPSCoordinates:  HotelGPS{Latitude: 3.0, Longitude: 4.0},
			RatePerNight:    HotelRateDetail{ExtractedLowest: new(200.0)},
		},
	}

	props := mapHotelProperties(sp, "USD")
	if len(props) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(props))
	}
	if props[0].ID != "p1" || props[0].Name != "Hotel One" {
		t.Errorf("props[0]: got ID=%q Name=%q", props[0].ID, props[0].Name)
	}
	if props[1].ID != "p2" || props[1].Name != "Hotel Two" {
		t.Errorf("props[1]: got ID=%q Name=%q", props[1].ID, props[1].Name)
	}
}

func TestMapHotelSearchDomainResponse_MatchingResults(t *testing.T) {
	hClass := 4
	serpResp := &HotelSearchResponse{
		SearchInformation: HotelSearchInfo{
			HotelsResultsState: "Results found",
		},
		Properties: []HotelProperty{
			{
				PropertyToken:      "h-1",
				Type:               "hotel",
				Name:               "Test Hotel",
				GPSCoordinates:     HotelGPS{Latitude: 10.0, Longitude: 20.0},
				ExtractedHotelClass: &hClass,
				RatePerNight:       HotelRateDetail{ExtractedLowest: new(120.0)},
			},
		},
		Brands: nil,
	}

	resp := mapHotelSearchDomainResponse(serpResp, "USD", false)

	if resp.Type != "hotels" {
		t.Errorf("Type: got %q, want hotels", resp.Type)
	}
	if resp.ResultsState != "matching" {
		t.Errorf("ResultsState: got %q, want matching", resp.ResultsState)
	}
	if len(resp.Properties) != 1 {
		t.Fatalf("expected 1 property, got %d", len(resp.Properties))
	}
	if resp.Properties[0].ID != "h-1" {
		t.Errorf("Properties[0].ID: got %q", resp.Properties[0].ID)
	}
	if resp.FromCache != false {
		t.Error("FromCache should be false")
	}
}

func TestMapHotelSearchDomainResponse_NonMatchingResults(t *testing.T) {
	serpResp := &HotelSearchResponse{
		SearchInformation: HotelSearchInfo{
			HotelsResultsState: "Non-matching results only",
		},
		Properties:            []HotelProperty{}, // ignored
		NonMatchingProperties: []HotelProperty{
			{
				PropertyToken:  "nm-1",
				Type:           "hotel",
				Name:           "Non-Matching Hotel",
				GPSCoordinates: HotelGPS{Latitude: 30.0, Longitude: 40.0},
				RatePerNight:   HotelRateDetail{ExtractedLowest: new(80.0)},
			},
		},
	}

	resp := mapHotelSearchDomainResponse(serpResp, "EUR", false)

	if resp.ResultsState != "non_matching_only" {
		t.Errorf("ResultsState: got %q, want non_matching_only", resp.ResultsState)
	}
	if len(resp.Properties) != 1 {
		t.Fatalf("expected 1 property from NonMatchingProperties, got %d", len(resp.Properties))
	}
	if resp.Properties[0].ID != "nm-1" {
		t.Errorf("Properties[0].ID: got %q, want nm-1", resp.Properties[0].ID)
	}
}

func TestMapHotelDetailsDomainResponse(t *testing.T) {
	detail := &HotelPropertyDetail{
		HotelProperty: HotelProperty{
			PropertyToken:   "det-1",
			Type:            "hotel",
			Name:            "Detailed Hotel",
			GPSCoordinates:  HotelGPS{Latitude: 51.5074, Longitude: -0.1278},
			ExtractedHotelClass: new(5),
			OverallRating:       new(4.7),
			Reviews:             new(500),
			RatePerNight:  HotelRateDetail{ExtractedLowest: new(250.0)},
			TotalRate:     HotelRateDetail{ExtractedLowest: new(750.0)},
			Images:        nil,
			Amenities:     []string{"spa"},
		},
		Address:       new("123 Main St, London"),
		DirectionsURL: new("https://maps.example.com"),
		TypicalPriceRange: &HotelTypicalPriceRange{
			ExtractedLowest:  200.0,
			ExtractedHighest: 400.0,
		},
		OtherReviews:     nil,
		HealthAndSafety:  nil,
		Sustainability:   nil,
	}

	resp := mapHotelDetailsDomainResponse(detail, "GBP")

	if resp.ID != "det-1" {
		t.Errorf("ID: got %q, want det-1", resp.ID)
	}
	if resp.Name != "Detailed Hotel" {
		t.Errorf("Name: got %q", resp.Name)
	}
	if resp.GPS.Lat != 51.5074 || resp.GPS.Lng != -0.1278 {
		t.Errorf("GPS: got (%.4f, %.4f)", resp.GPS.Lat, resp.GPS.Lng)
	}
	if resp.Address == nil || *resp.Address != "123 Main St, London" {
		t.Errorf("Address: got %v", resp.Address)
	}
	if resp.DirectionsURL == nil || *resp.DirectionsURL != "https://maps.example.com" {
		t.Errorf("DirectionsURL: got %v", resp.DirectionsURL)
	}
	if resp.PriceRange == nil {
		t.Fatal("PriceRange should be populated")
	}
	if resp.PriceRange.Currency != "GBP" {
		t.Errorf("PriceRange.Currency: got %q, want GBP", resp.PriceRange.Currency)
	}
	if resp.PriceRange.Min != 200.0 || resp.PriceRange.Max != 400.0 {
		t.Errorf("PriceRange: got (%.0f, %.0f), want (200, 400)", resp.PriceRange.Min, resp.PriceRange.Max)
	}
	if resp.Price.Currency != "GBP" {
		t.Errorf("Price.Currency: got %q, want GBP", resp.Price.Currency)
	}
	if resp.Price.PerNight.Amount != 250.0 {
		t.Errorf("Price.PerNight.Amount: got %f, want 250.0", resp.Price.PerNight.Amount)
	}
	if resp.FromCache != false {
		t.Error("FromCache should be false")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func ptr[T any](v T) *T {
	return &v
}
