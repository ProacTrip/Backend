// Adaptador SerpAPI para hoteles y vacation rentals.
// Implementa búsqueda y detalles via google_hotels engine.
package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// DTOs de Respuesta SerpAPI — google_hotels
// =============================================================================

// HotelSearchResponse is the top-level SerpAPI google_hotels search response.
type HotelSearchResponse struct {
	SearchInformation     HotelSearchInfo    `json:"search_information"`
	Properties            []HotelProperty    `json:"properties"`
	NonMatchingProperties []HotelProperty    `json:"non_matching_properties"`
	Brands                []HotelBrand       `json:"brands"`
	SerpapiPagination     *SerpapiPagination `json:"serpapi_pagination"`
}

// SerpapiPagination holds the pagination metadata from SerpAPI's raw response.
type SerpapiPagination struct {
	Next          string `json:"next"`
	NextPageToken string `json:"next_page_token"`
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
	// HotelClass *string is the raw string class from SerpAPI — unused, mapped via ExtractedHotelClass
	ExtractedHotelClass *int            `json:"extracted_hotel_class,omitempty"`
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
	Ratings           []HotelRating      `json:"ratings,omitempty"`
	ReviewsBreakdown  []HotelReviewBreakdown `json:"reviews_breakdown,omitempty"`
	// Hotel-only
	FreeCancellation *bool `json:"free_cancellation,omitempty"`
	SpecialOffer     *bool `json:"special_offer,omitempty"`
	EcoCertified     *bool `json:"eco_certified,omitempty"`
	// VR-only
	ExcludedAmenities []string       `json:"excluded_amenities,omitempty"`
	EssentialInfo     EssentialInfoField `json:"essential_info,omitempty"`
	Prices            []HotelPriceSource  `json:"prices,omitempty"`
}

// HotelPropertyDetail is the extended single-property response for hotel details.
type HotelPropertyDetail struct {
	HotelProperty            // embed base property
	Address          *string                 `json:"address,omitempty"`
	DirectionsURL    *string                 `json:"directions,omitempty"`
	TypicalPriceRange *HotelTypicalPriceRange `json:"typical_price_range,omitempty"`
	OtherReviews     []HotelOtherReview      `json:"other_reviews,omitempty"`
	HealthAndSafety  *HealthAndSafetyObject  `json:"health_and_safety,omitempty"`
	Sustainability   *SustainabilityObject   `json:"sustainability,omitempty"`
}

// HotelGPS holds latitude and longitude from SerpAPI.
type HotelGPS struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// HotelRateDetail holds rate information from SerpAPI.
type HotelRateDetail struct {
	Lowest                   *string  `json:"lowest,omitempty"`
	ExtractedLowest          *float64 `json:"extracted_lowest,omitempty"`
	BeforeTaxesFees          *string  `json:"before_taxes_fees,omitempty"`
	ExtractedBeforeTaxesFees *float64 `json:"extracted_before_taxes_fees,omitempty"`
}

// HotelImage holds image URLs from SerpAPI.
type HotelImage struct {
	Thumbnail     string `json:"thumbnail"`
	OriginalImage string `json:"original_image"`
}

// HotelNearbyPlace represents a nearby POI from SerpAPI.
type HotelNearbyPlace struct {
	Name            string                `json:"name"`
	Category        *string               `json:"category,omitempty"`
	Description     *string               `json:"description,omitempty"`
	Rating          *float64              `json:"rating,omitempty"`
	Reviews         *int                  `json:"reviews,omitempty"`
	Thumbnail       *string               `json:"thumbnail,omitempty"`
	Link            *string               `json:"link,omitempty"`
	GPSCoordinates  *HotelGPS             `json:"gps_coordinates,omitempty"`
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

// EssentialInfoField handles SerpAPI's inconsistent essential_info format.
// It can be a JSON array of key-value objects (old format) or a plain string (new format).
type EssentialInfoField []HotelEssentialKV

// UnmarshalJSON implements json.Unmarshaler to handle both array and string formats
// for the essential_info field returned by SerpAPI.
func (e *EssentialInfoField) UnmarshalJSON(data []byte) error {
	// Case 1: array of key-value objects [{key: "...", value: "..."}]
	var kvs []HotelEssentialKV
	if err := json.Unmarshal(data, &kvs); err == nil {
		*e = EssentialInfoField(kvs)
		return nil
	}

	// Case 2: array of plain strings ["Villa completa", "Capacidad para 2", ...]
	var strArr []string
	if err := json.Unmarshal(data, &strArr); err == nil {
		result := make(EssentialInfoField, 0, len(strArr))
		for _, s := range strArr {
			kv := parseEssentialInfoString(s)
			result = append(result, kv)
		}
		*e = result
		return nil
	}

	// Case 3: single string (legacy format — log and return empty)
	var rawStr string
	if err := json.Unmarshal(data, &rawStr); err != nil {
		return fmt.Errorf("essential_info: expected array of objects, array of strings, or string, got: %s", string(data))
	}

	slog.Warn("SerpAPI essential_info returned as string instead of array",
		slog.String("raw_value", rawStr),
	)

	*e = nil
	return nil
}

// HotelBrand represents a brand from SerpAPI hotel search.
type HotelBrand struct {
	ID     int          `json:"id"`
	Name   string       `json:"name"`
	Chains []HotelChain `json:"children"`
}

// HotelChain represents a brand chain from SerpAPI.
type HotelChain struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// =============================================================================
// DTOs Auxiliares — google_hotels
// =============================================================================

// HotelRating represents a star rating distribution bucket from SerpAPI's ratings array.
type HotelRating struct {
	Stars int `json:"stars"`
	Count int `json:"count"`
}

// HotelReviewBreakdown represents a review category from SerpAPI's reviews_breakdown.
type HotelReviewBreakdown struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	TotalMentioned int    `json:"total_mentioned"`
	Positive       int    `json:"positive"`
	Negative       int    `json:"negative"`
	Neutral        int    `json:"neutral"`
	CategoryToken  string `json:"category_token,omitempty"`
	SerpapiLink    string `json:"serpapi_link,omitempty"`
}

// HotelPriceSource represents a pricing option from a specific OTA in VR results.
type HotelPriceSource struct {
	Source       string           `json:"source"`
	Logo         string           `json:"logo,omitempty"`
	Link         string           `json:"link,omitempty"`
	NumGuests    *int             `json:"num_guests,omitempty"`
	RatePerNight *HotelRateDetail `json:"rate_per_night,omitempty"`
	TotalRate    *HotelRateDetail `json:"total_rate,omitempty"`
}

// HotelTypicalPriceRange matches SerpAPI's typical_price_range object in hotel details.
type HotelTypicalPriceRange struct {
	ExtractedLowest  float64 `json:"extracted_lowest"`
	ExtractedHighest float64 `json:"extracted_highest"`
}

// HotelOtherReview matches SerpAPI's other_reviews array in hotel details.
type HotelOtherReview struct {
	Source       string             `json:"source"`
	SourceIcon   string             `json:"source_icon,omitempty"`
	SourceRating *HotelRatingScore  `json:"source_rating,omitempty"`
	Reviews      int                `json:"reviews"`
	UserReview   *HotelUserReview   `json:"user_review,omitempty"`
	SourceNumber int                `json:"source_number,omitempty"`
	SerpapiLink  string             `json:"serpapi_link,omitempty"`
}

// HotelRatingScore is a score with a max value (used in source_rating and user_review.rating).
type HotelRatingScore struct {
	Score    float64 `json:"score"`
	MaxScore float64 `json:"max_score"`
}

// HotelUserReview is a featured user review within an other_reviews entry.
type HotelUserReview struct {
	Username string            `json:"username"`
	Date     string            `json:"date"`
	Rating   *HotelRatingScore `json:"rating"`
	Comment  string            `json:"comment"`
	URL      string            `json:"url,omitempty"`
}

// HealthAndSafetyObject matches SerpAPI's health_and_safety field in hotel/VR details.
type HealthAndSafetyObject struct {
	Groups      []HealthAndSafetyGroup `json:"groups"`
	DetailsLink string                 `json:"details_link,omitempty"`
}

// HealthAndSafetyGroup is a category group within health_and_safety.
type HealthAndSafetyGroup struct {
	Title string                `json:"title"`
	List  []HealthAndSafetyItem `json:"list"`
}

// HealthAndSafetyItem is a single health/safety measure.
type HealthAndSafetyItem struct {
	Title     string `json:"title"`
	Label     string `json:"label,omitempty"`
	Available bool   `json:"available"`
}

// SustainabilityObject matches SerpAPI's sustainability field in hotel/VR details.
type SustainabilityObject struct {
	Groups      []SustainabilityGroup `json:"groups"`
	DetailsLink string                `json:"details_link,omitempty"`
}

// SustainabilityGroup is a category group within sustainability.
type SustainabilityGroup struct {
	Title string               `json:"title"`
	List  []SustainabilityItem `json:"list"`
}

// SustainabilityItem is a single sustainability measure.
type SustainabilityItem struct {
	Title     string `json:"title"`
	Label     string `json:"label,omitempty"`
	Available bool   `json:"available"`
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
	Query           string
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

// FetchHotels performs a hotel/vacation rental search via SerpAPI and returns raw DTOs.
func (a *Adapter) FetchHotels(ctx context.Context, params HotelSearchParams) (*HotelSearchResponse, error) {
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

// FetchHotelDetail retrieves full details for a single property via SerpAPI.
func (a *Adapter) FetchHotelDetail(ctx context.Context, params HotelDetailsParams) (*HotelPropertyDetail, error) {
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
// HotelProvider — interface satisfaction (hotel search + details)
// =============================================================================

// SearchHotels satisfies domain.HotelProvider.
// Converts a domain request to SerpAPI params, calls the internal SerpAPI logic,
// and maps the raw response to domain types.
func (a *Adapter) SearchHotels(ctx context.Context, req domain.HotelSearchRequest) (*domain.HotelSearchResponse, error) {
	params := domainRequestToHotelParams(req)
	serpResp, err := a.FetchHotels(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapHotelSearchDomainResponse(serpResp, ptrStrDomain(req.Currency), req.VacationRentals), nil
}

// GetHotelDetails satisfies domain.HotelProvider.
func (a *Adapter) GetHotelDetails(ctx context.Context, req domain.HotelDetailsRequest) (*domain.HotelDetailsResponse, error) {
	params := domainRequestToDetailsParams(req)
	detail, err := a.FetchHotelDetail(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapHotelDetailsDomainResponse(detail, ptrStrDomain(req.Currency)), nil
}

// =============================================================================
// Mapeo Domain → SerpAPI Params
// =============================================================================

func domainRequestToHotelParams(req domain.HotelSearchRequest) HotelSearchParams {
	return HotelSearchParams{
		Query:            req.Query,
		GL:               ptrStrDomain(req.GL),
		HL:               ptrStrDomain(req.HL),
		Currency:         ptrStrDomain(req.Currency),
		CheckInDate:      req.CheckInDate,
		CheckOutDate:     req.CheckOutDate,
		Adults:           req.Adults,
		Children:         req.Children,
		ChildrenAges:     req.ChildrenAges,
		SortBy:           req.SortBy,
		MinPrice:         req.MinPrice,
		MaxPrice:         req.MaxPrice,
		PropertyTypes:    req.PropertyTypes,
		Amenities:        req.Amenities,
		VacationRentals:  req.VacationRentals,
		Rating:           req.Rating,
		HotelClasses:     req.HotelClasses,
		Brands:           req.Brands,
		FreeCancellation: req.FreeCancellation,
		SpecialOffers:    req.SpecialOffers,
		EcoCertified:     req.EcoCertified,
		Bedrooms:         req.Bedrooms,
		Bathrooms:        req.Bathrooms,
		PageToken:        req.PageToken,
	}
}

func domainRequestToDetailsParams(req domain.HotelDetailsRequest) HotelDetailsParams {
	return HotelDetailsParams{
		PropertyToken:   req.ID,
		Query:           req.Query,
		CheckInDate:     req.CheckInDate,
		CheckOutDate:    req.CheckOutDate,
		Adults:          req.Adults,
		Children:        req.Children,
		ChildrenAges:    req.ChildrenAges,
		GL:              ptrStrDomain(req.GL),
		HL:              ptrStrDomain(req.HL),
		Currency:        ptrStrDomain(req.Currency),
		VacationRentals: req.VacationRentals,
	}
}

// =============================================================================
// Mapeo SerpAPI DTO → Domain Response (Search)
// =============================================================================

func mapHotelSearchDomainResponse(serpResp *HotelSearchResponse, currency string, vacationRentals bool) *domain.HotelSearchResponse {
	respType := "hotels"
	if vacationRentals {
		respType = "vacation_rentals"
	}
	resp := &domain.HotelSearchResponse{
		Type:         respType,
		ResultsState: "matching",
	}

	if serpResp.SearchInformation.HotelsResultsState == "Non-matching results only" {
		resp.ResultsState = "non_matching_only"
		resp.Properties = mapHotelProperties(serpResp.NonMatchingProperties, currency)
	} else {
		resp.Properties = mapHotelProperties(serpResp.Properties, currency)
	}

	resp.Brands = mapHotelBrands(serpResp.Brands)

	// Map pagination from SerpAPI's serpapi_pagination block
	if serpResp.SerpapiPagination != nil {
		resp.Pagination = domain.HotelPagination{
			NextToken: serpResp.SerpapiPagination.NextPageToken,
			HasMore:   serpResp.SerpapiPagination.Next != "" || serpResp.SerpapiPagination.NextPageToken != "",
		}
	}

	resp.FromCache = false

	return resp
}

func mapHotelProperties(serpProps []HotelProperty, currency string) []domain.HotelProperty {
	if serpProps == nil {
		return nil
	}
	props := make([]domain.HotelProperty, 0, len(serpProps))
	for _, sp := range serpProps {
		props = append(props, mapSingleHotelProperty(sp, currency))
	}
	return props
}

func mapSingleHotelProperty(sp HotelProperty, currency string) domain.HotelProperty {
	p := domain.HotelProperty{
		ID:          sp.PropertyToken,
		Type:        sp.Type,
		Name:        sp.Name,
		Description: sp.Description,
		BookingURL:  sp.Link,
		GPS: domain.GPS{
			Lat: sp.GPSCoordinates.Latitude,
			Lng: sp.GPSCoordinates.Longitude,
		},
		HotelClass: sp.ExtractedHotelClass,
		CheckIn:    sp.CheckInTime,
		CheckOut:   sp.CheckOutTime,
		Rating: domain.HotelPropertyRating{
			Overall:  sp.OverallRating,
			Location: sp.LocationRating,
		},
		TotalReviews: sp.Reviews,
		Price: domain.HotelPrice{
			Currency: currency,
			PerNight: MapPriceDetail(sp.RatePerNight),
			Total:    MapPriceDetail(sp.TotalRate),
		},
		Images:            MapImages(sp.Images),
		Amenities:         sp.Amenities,
		NearbyPlaces:      MapNearbyPlaces(sp.NearbyPlaces),
		Ratings:           MapRatings(sp.Ratings),
		ReviewsBreakdown:  MapReviewsBreakdown(sp.ReviewsBreakdown),
		Prices:            mapHotelPrices(sp.Prices),
		FreeCancellation:  sp.FreeCancellation,
		SpecialOffer:      sp.SpecialOffer,
		EcoCertified:      sp.EcoCertified,
	}

	if sp.Type == "vacation_rental" {
		p.ExcludedAmenities = sp.ExcludedAmenities
		p.Capacity = MapCapacity([]HotelEssentialKV(sp.EssentialInfo))
	}

	return p
}

func mapHotelBrands(serpBrands []HotelBrand) []domain.HotelBrand {
	if serpBrands == nil {
		return nil
	}
	brands := make([]domain.HotelBrand, len(serpBrands))
	for i, sb := range serpBrands {
		b := domain.HotelBrand{
			ID:   sb.ID,
			Name: sb.Name,
		}
		if len(sb.Chains) > 0 {
			b.Chains = make([]domain.HotelBrandChain, len(sb.Chains))
			for j, sc := range sb.Chains {
				b.Chains[j] = domain.HotelBrandChain{
					ID:   sc.ID,
					Name: sc.Name,
				}
			}
		}
		brands[i] = b
	}
	return brands
}

func mapHotelPrices(serpPrices []HotelPriceSource) []domain.HotelPriceSource {
	if serpPrices == nil {
		return nil
	}
	out := make([]domain.HotelPriceSource, len(serpPrices))
	for i, p := range serpPrices {
		ps := domain.HotelPriceSource{
			NumGuests: p.NumGuests,
		}
		if p.RatePerNight != nil {
			pd := MapPriceDetail(*p.RatePerNight)
			ps.RatePerNight = &pd
		}
		out[i] = ps
	}
	return out
}

// =============================================================================
// Mapeo SerpAPI DTO → Domain Response (Details)
// =============================================================================

func mapHotelDetailsDomainResponse(detail *HotelPropertyDetail, currency string) *domain.HotelDetailsResponse {
	p := detail.HotelProperty
	resp := &domain.HotelDetailsResponse{
		ID:          p.PropertyToken,
		Type:        p.Type,
		Name:        p.Name,
		Description: p.Description,
		BookingURL:  p.Link,
		GPS: domain.GPS{
			Lat: p.GPSCoordinates.Latitude,
			Lng: p.GPSCoordinates.Longitude,
		},
		HotelClass: p.ExtractedHotelClass,
		CheckIn:    p.CheckInTime,
		CheckOut:   p.CheckOutTime,
		Rating: domain.HotelPropertyRating{
			Overall:  p.OverallRating,
			Location: p.LocationRating,
		},
		TotalReviews: p.Reviews,
		Price: domain.HotelPrice{
			Currency: currency,
			PerNight: MapPriceDetail(p.RatePerNight),
			Total:    MapPriceDetail(p.TotalRate),
		},
		Images:            MapImages(p.Images),
		Amenities:         p.Amenities,
		NearbyPlaces:      MapNearbyPlaces(p.NearbyPlaces),
		Address:           detail.Address,
		DirectionsURL:     detail.DirectionsURL,
		PriceRange:        mapTypicalPriceRange(detail.TypicalPriceRange, currency),
		ExternalReviews:   mapExternalReviews(detail.OtherReviews),
		HealthAndSafety:   mapHealthAndSafety(detail.HealthAndSafety),
		Sustainability:    mapSustainability(detail.Sustainability),
		Ratings:           MapRatings(p.Ratings),
		ReviewsBreakdown:  MapReviewsBreakdown(p.ReviewsBreakdown),
		FreeCancellation:  p.FreeCancellation,
		SpecialOffer:      p.SpecialOffer,
		EcoCertified:      p.EcoCertified,
		FromCache:         false,
	}

	if p.Type == "vacation_rental" {
		resp.ExcludedAmenities = p.ExcludedAmenities
		resp.Capacity = MapCapacity([]HotelEssentialKV(p.EssentialInfo))
	}

	return resp
}

func mapTypicalPriceRange(tpr *HotelTypicalPriceRange, currency string) *domain.HotelPriceRange {
	if tpr == nil {
		return nil
	}
	return &domain.HotelPriceRange{
		Currency: currency,
		Min:      tpr.ExtractedLowest,
		Max:      tpr.ExtractedHighest,
	}
}

func mapExternalReviews(serpReviews []HotelOtherReview) []domain.HotelExternalReview {
	if serpReviews == nil {
		return nil
	}
	out := make([]domain.HotelExternalReview, len(serpReviews))
	for i, sr := range serpReviews {
		or := domain.HotelExternalReview{
			Source:       sr.Source,
			LogoURL:      sr.SourceIcon,
			TotalReviews: sr.Reviews,
		}
		if sr.SourceRating != nil {
			or.Score = sr.SourceRating.Score
			or.MaxScore = sr.SourceRating.MaxScore
		}
		if sr.UserReview != nil {
			fr := domain.HotelFeaturedReview{
				Author:  sr.UserReview.Username,
				Date:    sr.UserReview.Date,
				Comment: sr.UserReview.Comment,
			}
			if sr.UserReview.Rating != nil {
				fr.Score = sr.UserReview.Rating.Score
			}
			if sr.UserReview.URL != "" {
				fr.URL = new(sr.UserReview.URL)
			}
			or.FeaturedReview = &fr
		}
		out[i] = or
	}
	return out
}

func mapHealthAndSafety(hs *HealthAndSafetyObject) []domain.HotelHealthSafetyCategory {
	if hs == nil {
		return nil
	}
	groups := make([]domain.HotelHealthSafetyCategory, len(hs.Groups))
	for i, g := range hs.Groups {
		items := make([]domain.HotelHealthSafetyItem, len(g.List))
		for j, item := range g.List {
			items[j] = domain.HotelHealthSafetyItem{
				Name:      item.Title,
				Available: item.Available,
			}
		}
		groups[i] = domain.HotelHealthSafetyCategory{
			Category: g.Title,
			Items:    items,
		}
	}
	return groups
}

func mapSustainability(s *SustainabilityObject) []domain.HotelSustainabilityCategory {
	if s == nil {
		return nil
	}
	groups := make([]domain.HotelSustainabilityCategory, len(s.Groups))
	for i, g := range s.Groups {
		items := make([]domain.HotelSustainabilityItem, len(g.List))
		for j, item := range g.List {
			items[j] = domain.HotelSustainabilityItem{
				Name:      item.Title,
				Available: item.Available,
			}
		}
		groups[i] = domain.HotelSustainabilityCategory{
			Category: g.Title,
			Items:    items,
		}
	}
	return groups
}

// ptrStrDomain returns the dereferenced string, or "" if nil.
func ptrStrDomain(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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

	// Internal SerpAPI params (always set, not exposed to clients)
	params["no_cache"] = "true" // Always fetch fresh; we handle cache ourselves

	return params
}

func buildHotelDetailsParams(p HotelDetailsParams) map[string]string {
	params := make(map[string]string)

	params["engine"] = "google_hotels"
	params["property_token"] = p.PropertyToken

	if p.Query != "" {
		params["q"] = p.Query
	}
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

	// Internal SerpAPI params (always set, not exposed to clients)
	params["no_cache"] = "true" // Always fetch fresh; we handle cache ourselves

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

// parseEssentialInfoString parses a single essential_info string into key-value pairs.
// Examples:
//
//	"Villa completa" → {Key: "unit_type", Value: "Villa completa"}
//	"Capacidad para 2" → {Key: "guests", Value: "2"}
//	"1 dormitorio" → {Key: "bedrooms", Value: "1"}
//	"2.153 ft²" → {Key: "area", Value: "2.153 ft²"}
func parseEssentialInfoString(s string) HotelEssentialKV {
	// Spanish patterns
	if strings.Contains(strings.ToLower(s), "villa completa") || strings.Contains(strings.ToLower(s), "casa") || strings.Contains(strings.ToLower(s), "entire") {
		return HotelEssentialKV{Key: "unit_type", Value: s}
	}
	if strings.Contains(strings.ToLower(s), "capacidad para") || strings.Contains(strings.ToLower(s), "guests") || strings.Contains(strings.ToLower(s), "sleeps") {
		re := regexp.MustCompile(`\d+`)
		num := re.FindString(s)
		return HotelEssentialKV{Key: "guests", Value: num}
	}
	if strings.Contains(strings.ToLower(s), "dormitorio") || strings.Contains(strings.ToLower(s), "bedroom") {
		re := regexp.MustCompile(`\d+`)
		num := re.FindString(s)
		return HotelEssentialKV{Key: "bedrooms", Value: num}
	}
	if strings.Contains(strings.ToLower(s), "baño") || strings.Contains(strings.ToLower(s), "bathroom") {
		re := regexp.MustCompile(`\d+`)
		num := re.FindString(s)
		return HotelEssentialKV{Key: "bathrooms", Value: num}
	}
	if strings.Contains(strings.ToLower(s), "cama") || strings.Contains(strings.ToLower(s), "bed") {
		re := regexp.MustCompile(`\d+`)
		num := re.FindString(s)
		return HotelEssentialKV{Key: "beds", Value: num}
	}
	if strings.Contains(s, "ft²") || strings.Contains(s, "m²") || strings.Contains(s, "sq") {
		return HotelEssentialKV{Key: "area", Value: s}
	}
	return HotelEssentialKV{} // unknown — mapCapacity ignores empty Key
}
