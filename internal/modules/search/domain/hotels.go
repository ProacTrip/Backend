// Domain entities y tipos de negocio para búsqueda de hoteles.
// Define request, response, y todos los tipos relacionados con hoteles y vacation rentals.
package domain

// =============================================================================
// HotelSearchRequest
// =============================================================================

// HotelSearchRequest is the domain representation of a hotel search request.
type HotelSearchRequest struct {
	Query            string   `json:"query"`
	CheckInDate      string   `json:"check_in_date"`
	CheckOutDate     string   `json:"check_out_date"`
	Adults           int      `json:"adults"`
	Children         int      `json:"children"`
	ChildrenAges     []int    `json:"children_ages"`
	GL               *string  `json:"gl,omitzero"`
	HL               *string  `json:"hl,omitzero"`
	Currency         *string  `json:"currency,omitzero"`
	MinPrice         *float64 `json:"min_price"`
	MaxPrice         *float64 `json:"max_price"`
	SortBy           *int     `json:"sort_by"`
	Rating           *int     `json:"rating"`
	PropertyTypes    []int    `json:"property_types"`
	Amenities        []int    `json:"amenities"`
	VacationRentals  bool     `json:"vacation_rentals"`
	HotelClasses     []int    `json:"hotel_classes"`
	Brands           []int    `json:"brands"`
	FreeCancellation bool     `json:"free_cancellation"`
	SpecialOffers    bool     `json:"special_offers"`
	EcoCertified     bool     `json:"eco_certified"`
	Bedrooms         *int     `json:"bedrooms"`
	Bathrooms        *int     `json:"bathrooms"`
	PageToken        string   `json:"page_token"`
}

// =============================================================================
// HotelSearchResponse
// =============================================================================

// HotelSearchResponse is the domain hotel search response.
type HotelSearchResponse struct {
	Type         string          `json:"type"`
	ResultsState string          `json:"results_state"`
	Properties   []HotelProperty `json:"properties"`
	Brands       []HotelBrand    `json:"brands"`
	Pagination   HotelPagination `json:"pagination"`
	FromCache    bool            `json:"from_cache"`
	CachedAt     *string         `json:"cached_at,omitzero"`
}

// HotelProperty represents a single hotel or vacation rental in search results.
type HotelProperty struct {
	ID                string                        `json:"id"`
	Type              string                        `json:"type"`
	Name              string                        `json:"name"`
	Description       string                        `json:"description,omitzero"`
	BookingURL        string                        `json:"booking_url,omitzero"`
	GPS               GPS                           `json:"gps"`
	HotelClass        *int                          `json:"hotel_class,omitzero"`
	CheckIn           string                        `json:"check_in,omitzero"`
	CheckOut          string                        `json:"check_out,omitzero"`
	Rating            HotelPropertyRating           `json:"rating"`
	TotalReviews      *int                          `json:"total_reviews,omitzero"`
	Price             HotelPrice                    `json:"price"`
	Images            []Image                       `json:"images"`
	Amenities         []string                      `json:"amenities"`
	NearbyPlaces      []NearbyPlace                 `json:"nearby_places"`
	FreeCancellation  *bool                         `json:"free_cancellation,omitzero"`
	SpecialOffer      *bool                         `json:"special_offer,omitzero"`
	EcoCertified      *bool                         `json:"eco_certified,omitzero"`
	ExcludedAmenities []string                      `json:"excluded_amenities,omitzero"`
	Capacity          *Capacity                     `json:"capacity,omitzero"`
	Ratings           []HotelRatingResponse         `json:"ratings,omitzero"`
	ReviewsBreakdown  []HotelReviewBreakdownResponse `json:"reviews_breakdown,omitzero"`
	Prices            []HotelPriceSource            `json:"prices,omitzero"`
}

// HotelPropertyRating holds overall and location ratings for a property.
type HotelPropertyRating struct {
	Overall  *float64 `json:"overall,omitzero"`
	Location *float64 `json:"location,omitzero"`
}

// HotelPrice holds per-night and total price details.
type HotelPrice struct {
	Currency string      `json:"currency"`
	PerNight PriceDetail `json:"per_night"`
	Total    PriceDetail `json:"total"`
}

// HotelBrand represents a hotel brand.
type HotelBrand struct {
	ID     int               `json:"id"`
	Name   string            `json:"name"`
	Chains []HotelBrandChain `json:"chains"`
}

// HotelBrandChain represents a brand chain.
type HotelBrandChain struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// HotelPagination holds the next page token and whether there are more results.
type HotelPagination struct {
	NextToken string `json:"next_token"`
	HasMore   bool   `json:"has_more"`
}

// HotelPriceSource represents pricing from a specific OTA in VR results.
type HotelPriceSource struct {
	Source       string       `json:"source"`
	Logo         string       `json:"logo,omitzero"`
	NumGuests    *int         `json:"num_guests,omitzero"`
	RatePerNight *PriceDetail `json:"rate_per_night,omitzero"`
}

// =============================================================================
// HotelDetailsRequest
// =============================================================================

// HotelDetailsRequest is the domain representation of a hotel details request.
type HotelDetailsRequest struct {
	ID              string  `json:"id"`
	CheckInDate     string  `json:"check_in_date"`
	CheckOutDate    string  `json:"check_out_date"`
	Adults          int     `json:"adults"`
	Children        int     `json:"children"`
	ChildrenAges    []int   `json:"children_ages"`
	GL              *string `json:"gl,omitzero"`
	HL              *string `json:"hl,omitzero"`
	Currency        *string `json:"currency,omitzero"`
	VacationRentals bool    `json:"vacation_rentals"`
}

// =============================================================================
// HotelDetailsResponse
// =============================================================================

// HotelDetailsResponse is the domain hotel details response.
// Contains both hotel-specific and VR-specific fields.
type HotelDetailsResponse struct {
	ID                 string                              `json:"id"`
	Type               string                              `json:"type"`
	Name               string                              `json:"name"`
	Description        string                              `json:"description,omitzero"`
	BookingURL         string                              `json:"booking_url,omitzero"`
	GPS                GPS                                 `json:"gps"`
	HotelClass         *int                                `json:"hotel_class,omitzero"`
	CheckIn            string                              `json:"check_in,omitzero"`
	CheckOut           string                              `json:"check_out,omitzero"`
	Rating             HotelPropertyRating                 `json:"rating"`
	TotalReviews       *int                                `json:"total_reviews,omitzero"`
	Price              HotelPrice                          `json:"price"`
	Images             []Image                             `json:"images"`
	Amenities          []string                            `json:"amenities"`
	NearbyPlaces       []NearbyPlace                       `json:"nearby_places"`
	Address            *string                             `json:"address,omitzero"`
	DirectionsURL      *string                             `json:"directions_url,omitzero"`
	PriceRange         *HotelPriceRange                    `json:"price_range,omitzero"`
	ExternalReviews    []HotelExternalReview               `json:"external_reviews,omitzero"`
	HealthAndSafety    []HotelHealthSafetyCategory         `json:"health_and_safety,omitzero"`
	Sustainability     []HotelSustainabilityCategory       `json:"sustainability,omitzero"`
	Ratings            []HotelRatingResponse               `json:"ratings,omitzero"`
	ReviewsBreakdown   []HotelReviewBreakdownResponse      `json:"reviews_breakdown,omitzero"`
	FreeCancellation   *bool                               `json:"free_cancellation,omitzero"`
	SpecialOffer       *bool                               `json:"special_offer,omitzero"`
	EcoCertified       *bool                               `json:"eco_certified,omitzero"`
	ExcludedAmenities  []string                            `json:"excluded_amenities,omitzero"`
	Capacity           *Capacity                           `json:"capacity,omitzero"`
	FromCache          bool                                `json:"from_cache"`
	CachedAt           *string                             `json:"cached_at,omitzero"`
}

// HotelPriceRange represents the typical price range for a hotel.
type HotelPriceRange struct {
	Currency string  `json:"currency"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
}

// HotelExternalReview holds a review from an external source.
type HotelExternalReview struct {
	Source         string                  `json:"source"`
	LogoURL        string                  `json:"logo_url,omitzero"`
	Score          float64                 `json:"score"`
	MaxScore       float64                 `json:"max_score"`
	TotalReviews   int                     `json:"total_reviews"`
	FeaturedReview *HotelFeaturedReview    `json:"featured_review,omitzero"`
}

// HotelFeaturedReview is a featured user review within an external review.
type HotelFeaturedReview struct {
	Author  string  `json:"author"`
	Date    string  `json:"date"`
	Score   float64 `json:"score"`
	Comment string  `json:"comment"`
	URL     *string `json:"url,omitzero"`
}

// HotelHealthSafetyCategory is a category group within health_and_safety.
type HotelHealthSafetyCategory struct {
	Category string                   `json:"category"`
	Items    []HotelHealthSafetyItem  `json:"items"`
}

// HotelHealthSafetyItem is a single health/safety measure.
type HotelHealthSafetyItem struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// HotelSustainabilityCategory is a category group within sustainability.
type HotelSustainabilityCategory struct {
	Category string                     `json:"category"`
	Items    []HotelSustainabilityItem  `json:"items"`
}

// HotelSustainabilityItem is a single sustainability measure.
type HotelSustainabilityItem struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}
