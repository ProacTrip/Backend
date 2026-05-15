// Domain entities y tipos de negocio para búsqueda de hoteles.
// Define request, response, y todos los tipos relacionados con hoteles y vacation rentals.
package domain

import "time"

// =============================================================================
// HotelSearchRequest
// =============================================================================

// HotelSearchRequest es la representación de dominio de una solicitud de búsqueda de hoteles.
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

// HotelSearchResponse es la respuesta de dominio para búsqueda de hoteles.
type HotelSearchResponse struct {
	Type         string          `json:"type"`
	ResultsState string          `json:"results_state"`
	Properties   []HotelProperty `json:"properties"`
	Brands       []HotelBrand    `json:"brands"`
	Pagination   HotelPagination `json:"pagination"`
	FromCache    bool             `json:"from_cache"`
	CachedAt     *time.Time       `json:"cached_at"`
}

// HotelProperty representa un hotel o vacation rental individual en resultados de búsqueda.
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

// HotelPropertyRating contiene ratings generales y de ubicación para una propiedad.
type HotelPropertyRating struct {
	Overall  *float64 `json:"overall,omitzero"`
	Location *float64 `json:"location,omitzero"`
}

// HotelPrice contiene detalles de precio por noche y total.
type HotelPrice struct {
	Currency string      `json:"currency"`
	PerNight PriceDetail `json:"per_night"`
	Total    PriceDetail `json:"total"`
}

// HotelBrand representa una marca de hotel.
type HotelBrand struct {
	ID     int               `json:"id"`
	Name   string            `json:"name"`
	Chains []HotelBrandChain `json:"chains"`
}

// HotelBrandChain representa una cadena de marca.
type HotelBrandChain struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// HotelPagination contiene el token de página siguiente y si hay más resultados.
type HotelPagination struct {
	NextToken string `json:"next_token"`
	HasMore   bool   `json:"has_more"`
}

// HotelPriceSource representa precios de una OTA específica en resultados VR.
type HotelPriceSource struct {
	Source       string       `json:"source"`
	Logo         string       `json:"logo,omitzero"`
	NumGuests    *int         `json:"num_guests,omitzero"`
	RatePerNight *PriceDetail `json:"rate_per_night,omitzero"`
}

// =============================================================================
// HotelDetailsRequest
// =============================================================================

// HotelDetailsRequest es la representación de dominio de una solicitud de detalles de hotel.
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

// HotelDetailsResponse es la respuesta de dominio para detalles de hotel.
// Contiene tanto campos específicos de hotel como de VR.
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
	Price              HotelPrice                          `json:"price,omitzero"`
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
	FromCache          bool                                 `json:"from_cache"`
	CachedAt           *time.Time                           `json:"cached_at"`
}

// HotelPriceRange representa el rango de precios típico para un hotel.
type HotelPriceRange struct {
	Currency string  `json:"currency"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
}

// HotelExternalReview contiene una reseña de una fuente externa.
type HotelExternalReview struct {
	Source         string                  `json:"source"`
	LogoURL        string                  `json:"logo_url,omitzero"`
	Score          float64                 `json:"score"`
	MaxScore       float64                 `json:"max_score"`
	TotalReviews   int                     `json:"total_reviews"`
	FeaturedReview *HotelFeaturedReview    `json:"featured_review,omitzero"`
}

// HotelFeaturedReview es una reseña de usuario destacada dentro de una reseña externa.
type HotelFeaturedReview struct {
	Author  string  `json:"author"`
	Date    string  `json:"date"`
	Score   float64 `json:"score"`
	Comment string  `json:"comment"`
	URL     *string `json:"url,omitzero"`
}

// HotelHealthSafetyCategory es un grupo de categoría dentro de health_and_safety.
type HotelHealthSafetyCategory struct {
	Category string                   `json:"category"`
	Items    []HotelHealthSafetyItem  `json:"items"`
}

// HotelHealthSafetyItem es una medida individual de salud/seguridad.
type HotelHealthSafetyItem struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// HotelSustainabilityCategory es un grupo de categoría dentro de sustainability.
type HotelSustainabilityCategory struct {
	Category string                     `json:"category"`
	Items    []HotelSustainabilityItem  `json:"items"`
}

// HotelSustainabilityItem es una medida individual de sostenibilidad.
type HotelSustainabilityItem struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}
