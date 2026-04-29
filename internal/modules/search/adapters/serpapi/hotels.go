// Adaptador SerpAPI para hoteles y vacation rentals.
// Implementa búsqueda y detalles via google_hotels engine.
package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// =============================================================================
// DTOs de Respuesta SerpAPI — google_hotels
// =============================================================================

// HotelSearchResponse is the top-level SerpAPI google_hotels search response.
type HotelSearchResponse struct {
	SearchInformation     HotelSearchInfo  `json:"search_information"`
	Properties            []HotelProperty  `json:"properties"`
	NonMatchingProperties []HotelProperty  `json:"non_matching_properties"`
	Brands                []HotelBrand     `json:"brands"`
}

// HotelSearchInfo contains search metadata from SerpAPI hotel response.
type HotelSearchInfo struct {
	HotelsResultsState string `json:"hotels_results_state"`
}

// HotelProperty maps a single property result from SerpAPI google_hotels.
type HotelProperty struct {
	Type              string           `json:"type"` // "hotel" or "vacation_rental"
	Name              string           `json:"name"`
	Description       string           `json:"description,omitempty"`
	Link              string           `json:"link,omitempty"`
	PropertyToken     string           `json:"property_token"`
	GPSCoordinates    HotelGPS         `json:"gps_coordinates"`
	HotelClass        *int             `json:"hotel_class,omitempty"`
	CheckInTime       string           `json:"check_in_time,omitempty"`
	CheckOutTime      string           `json:"check_out_time,omitempty"`
	OverallRating     *float64         `json:"overall_rating,omitempty"`
	LocationRating    *float64         `json:"location_rating,omitempty"`
	Reviews           *int             `json:"reviews,omitempty"`
	RatePerNight      HotelRateDetail  `json:"rate_per_night,omitempty"`
	TotalRate         HotelRateDetail  `json:"total_rate,omitempty"`
	Images            []HotelImage     `json:"images,omitempty"`
	Amenities         []string         `json:"amenities,omitempty"`
	NearbyPlaces      []HotelNearbyPlace `json:"nearby_places,omitempty"`
	// Hotel-only
	FreeCancellation *bool `json:"free_cancellation,omitempty"`
	SpecialOffer     *bool `json:"special_offer,omitempty"`
	EcoCertified     *bool `json:"eco_certified,omitempty"`
	// VR-only
	ExcludedAmenities []string       `json:"excluded_amenities,omitempty"`
	EssentialInfo     []HotelEssentialKV `json:"essential_info,omitempty"`
}

// HotelPropertyDetail is the extended single-property response for hotel details.
type HotelPropertyDetail struct {
	HotelProperty            // embed base property
	Address         *string  `json:"address,omitempty"`
	DirectionsURL   *string  `json:"directions_url,omitempty"`
	PriceRange      *string  `json:"price_range,omitempty"`
	ExternalReviews []HotelExternalReview `json:"external_reviews,omitempty"`
	HealthAndSafety *string  `json:"health_and_safety,omitempty"`
	Sustainability  *string  `json:"sustainability,omitempty"`
}

// HotelGPS holds latitude and longitude from SerpAPI.
type HotelGPS struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// HotelRateDetail holds rate information from SerpAPI.
type HotelRateDetail struct {
	Lowest          *float64 `json:"lowest,omitempty"`
	ExtractedLowest *float64 `json:"extracted_lowest,omitempty"`
	BeforeTaxesFees *float64 `json:"before_taxes_fees,omitempty"`
}

// HotelImage holds image URLs from SerpAPI.
type HotelImage struct {
	Thumbnail     string `json:"thumbnail"`
	OriginalImage string `json:"original_image"`
}

// HotelNearbyPlace represents a nearby POI from SerpAPI.
type HotelNearbyPlace struct {
	Name            string                `json:"name"`
	Transportations []HotelTransportation `json:"transportations,omitempty"`
}

// HotelTransportation represents a transport option from SerpAPI.
type HotelTransportation struct {
	Type     string `json:"type"`
	Duration string `json:"duration"`
}

// HotelEssentialKV is a key-value pair from essential_info in SerpAPI.
type HotelEssentialKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// HotelExternalReview is an external review from SerpAPI hotel details.
type HotelExternalReview struct {
	Source string  `json:"source"`
	Rating float64 `json:"rating"`
	Count  int     `json:"count"`
	Link   string  `json:"link,omitempty"`
}

// HotelBrand represents a brand from SerpAPI hotel search.
type HotelBrand struct {
	ID     int          `json:"id"`
	Name   string       `json:"name"`
	Chains []HotelChain `json:"chains"`
}

// HotelChain represents a brand chain from SerpAPI.
type HotelChain struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// =============================================================================
// Parámetros de Entrada del Adaptador
// =============================================================================

// HotelSearchParams bundles all parameters for the hotel search adapter call.
type HotelSearchParams struct {
	Query           string
	GL              string
	HL              string
	Currency        string
	CheckInDate     string
	CheckOutDate    string
	Adults          int
	Children        int
	ChildrenAges    []int
	SortBy          *int
	MinPrice        *float64
	MaxPrice        *float64
	PropertyTypes   []int
	Amenities       []int
	VacationRentals bool
	Rating          *int
	HotelClasses    []int
	Brands          []int
	FreeCancellation bool
	SpecialOffers    bool
	EcoCertified     bool
	Bedrooms        *int
	Bathrooms       *int
	PageToken       string
}

// HotelDetailsParams bundles all parameters for the hotel details adapter call.
type HotelDetailsParams struct {
	PropertyToken   string
	CheckInDate     string
	CheckOutDate    string
	Adults          int
	Children        int
	ChildrenAges    []int
	GL              string
	HL              string
	Currency        string
	VacationRentals bool
}

// =============================================================================
// SearchHotels — búsqueda de hoteles y vacation rentals
// =============================================================================

// SearchHotels performs a hotel/vacation rental search via SerpAPI and returns raw DTOs.
func (a *Adapter) SearchHotels(ctx context.Context, params HotelSearchParams) (*HotelSearchResponse, error) {
	serpParams := buildHotelSearchParams(params)
	raw, err := a.client.SearchHotels(ctx, serpParams)
	if err != nil {
		return nil, fmt.Errorf("serpapi hotel search: %w", err)
	}

	dto, err := convertToHotelSearchResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("serpapi parse hotel response: %w", err)
	}

	return dto, nil
}

// HotelDetails retrieves full details for a single property via SerpAPI.
func (a *Adapter) HotelDetails(ctx context.Context, params HotelDetailsParams) (*HotelPropertyDetail, error) {
	serpParams := buildHotelDetailsParams(params)
	raw, err := a.client.GetHotelDetails(ctx, serpParams)
	if err != nil {
		return nil, fmt.Errorf("serpapi hotel details: %w", err)
	}

	dto, err := convertToHotelDetailsResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("serpapi parse hotel details response: %w", err)
	}

	return dto, nil
}

// =============================================================================
// Conversión Raw → DTO
// =============================================================================

func convertToHotelSearchResponse(raw map[string]interface{}) (*HotelSearchResponse, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal serpapi hotel response: %w", err)
	}
	var dto HotelSearchResponse
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("unmarshal serpapi hotel response: %w", err)
	}
	return &dto, nil
}

func convertToHotelDetailsResponse(raw map[string]interface{}) (*HotelPropertyDetail, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal serpapi hotel details response: %w", err)
	}
	var dto HotelPropertyDetail
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("unmarshal serpapi hotel details response: %w", err)
	}
	return &dto, nil
}

// =============================================================================
// Construcción de Parámetros SerpAPI
// =============================================================================

func buildHotelSearchParams(p HotelSearchParams) map[string]string {
	params := make(map[string]string)

	params["engine"] = "google_hotels"

	// Required
	if p.Query != "" {
		params["q"] = p.Query
	}
	if p.CheckInDate != "" {
		params["check_in_date"] = p.CheckInDate
	}
	if p.CheckOutDate != "" {
		params["check_out_date"] = p.CheckOutDate
	}

	// Locale / currency
	if p.GL != "" {
		params["gl"] = p.GL
	}
	if p.HL != "" {
		params["hl"] = p.HL
	}
	if p.Currency != "" {
		params["currency"] = p.Currency
	}

	// Passengers
	if p.Adults > 0 {
		params["adults"] = itoa(p.Adults)
	}
	if p.Children > 0 {
		params["children"] = itoa(p.Children)
	}
	if len(p.ChildrenAges) > 0 {
		ages := make([]string, len(p.ChildrenAges))
		for i, a := range p.ChildrenAges {
			ages[i] = itoa(a)
		}
		params["children_ages"] = strings.Join(ages, ",")
	}

	// Sorting / price
	if p.SortBy != nil {
		params["sort_by"] = itoa(*p.SortBy)
	}
	if p.MinPrice != nil {
		params["min_price"] = fmt.Sprintf("%.0f", *p.MinPrice)
	}
	if p.MaxPrice != nil {
		params["max_price"] = fmt.Sprintf("%.0f", *p.MaxPrice)
	}

	// Filters
	if len(p.PropertyTypes) > 0 {
		params["property_types"] = joinInts(p.PropertyTypes)
	}
	if len(p.Amenities) > 0 {
		params["amenities"] = joinInts(p.Amenities)
	}
	if p.VacationRentals {
		params["vacation_rentals"] = "true"
	}
	if p.Rating != nil {
		params["rating"] = itoa(*p.Rating)
	}
	if len(p.HotelClasses) > 0 {
		params["hotel_class"] = joinInts(p.HotelClasses)
	}
	if len(p.Brands) > 0 {
		params["brands"] = joinInts(p.Brands)
	}
	if p.FreeCancellation {
		params["free_cancellation"] = "true"
	}
	if p.SpecialOffers {
		params["special_offers"] = "true"
	}
	if p.EcoCertified {
		params["eco_certified"] = "true"
	}
	if p.Bedrooms != nil {
		params["bedrooms"] = itoa(*p.Bedrooms)
	}
	if p.Bathrooms != nil {
		params["bathrooms"] = itoa(*p.Bathrooms)
	}

	// Pagination
	if p.PageToken != "" {
		params["page_token"] = p.PageToken
	}

	return params
}

func buildHotelDetailsParams(p HotelDetailsParams) map[string]string {
	params := make(map[string]string)

	params["engine"] = "google_hotels"
	params["property_token"] = p.PropertyToken

	if p.CheckInDate != "" {
		params["check_in_date"] = p.CheckInDate
	}
	if p.CheckOutDate != "" {
		params["check_out_date"] = p.CheckOutDate
	}

	if p.GL != "" {
		params["gl"] = p.GL
	}
	if p.HL != "" {
		params["hl"] = p.HL
	}
	if p.Currency != "" {
		params["currency"] = p.Currency
	}

	if p.Adults > 0 {
		params["adults"] = itoa(p.Adults)
	}
	if p.Children > 0 {
		params["children"] = itoa(p.Children)
	}
	if len(p.ChildrenAges) > 0 {
		ages := make([]string, len(p.ChildrenAges))
		for i, a := range p.ChildrenAges {
			ages[i] = itoa(a)
		}
		params["children_ages"] = strings.Join(ages, ",")
	}

	if p.VacationRentals {
		params["vacation_rentals"] = "true"
	}

	return params
}

// =============================================================================
// Helpers
// =============================================================================

func joinInts(vals []int) string {
	if len(vals) == 0 {
		return ""
	}
	strs := make([]string, len(vals))
	for i, v := range vals {
		strs[i] = itoa(v)
	}
	return strings.Join(strs, ",")
}
