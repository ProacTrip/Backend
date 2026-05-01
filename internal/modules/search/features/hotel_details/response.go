// DTO de respuesta para detalles de hotel.
// Contiene campos específicos de hotel y vacation rental.
package hotel_details

import "github.com/ProacTrip/Backend/internal/modules/search/domain"

// =============================================================================
// Shared type aliases (defined once in domain, reused across features)
// =============================================================================

type GPS = domain.GPS
type Transport = domain.Transport
type Image = domain.Image
type NearbyPlace = domain.NearbyPlace
type PriceDetail = domain.PriceDetail
type Capacity = domain.Capacity
type HotelRatingResponse = domain.HotelRatingResponse
type HotelReviewBreakdownResponse = domain.HotelReviewBreakdownResponse

// =============================================================================
// Response — DTO de salida para hotel details
// =============================================================================

// Response is the hotel details API response.
// Contains both hotel-specific and VR-specific fields.
type Response struct {
	ID                string           `json:"id"`
	Type              string           `json:"type"` // "hotel" or "vacation_rental"
	Name              string           `json:"name"`
	Description       string           `json:"description,omitempty"`
	BookingURL        string           `json:"booking_url,omitempty"`
	GPS               GPS              `json:"gps"`
	HotelClass        *int             `json:"hotel_class,omitempty"`
	CheckIn           string           `json:"check_in,omitempty"`
	CheckOut          string           `json:"check_out,omitempty"`
	Rating            Rating           `json:"rating"`
	TotalReviews      *int             `json:"total_reviews,omitempty"`
	Price             Price            `json:"price"`
	Images            []Image          `json:"images"`
	Amenities         []string         `json:"amenities"`
	NearbyPlaces      []NearbyPlace    `json:"nearby_places"`
	// Hotel-only detail fields
	Address          *string                         `json:"address,omitempty"`
	DirectionsURL    *string                         `json:"directions_url,omitempty"`
	PriceRange       *HotelPriceRangeResponse        `json:"price_range,omitempty"`
	ExternalReviews  []OtherReviewResponse           `json:"external_reviews,omitempty"`
	HealthAndSafety  []HealthAndSafetyCategoryResponse `json:"health_and_safety,omitempty"`
	Sustainability   []SustainabilityCategoryResponse  `json:"sustainability,omitempty"`
	Ratings          []HotelRatingResponse             `json:"ratings,omitempty"`
	ReviewsBreakdown []HotelReviewBreakdownResponse    `json:"reviews_breakdown,omitempty"`
	FreeCancellation *bool             `json:"free_cancellation,omitempty"`
	SpecialOffer     *bool             `json:"special_offer,omitempty"`
	EcoCertified     *bool             `json:"eco_certified,omitempty"`
	// VR-only
	ExcludedAmenities []string  `json:"excluded_amenities,omitempty"`
	Capacity          *Capacity `json:"capacity,omitempty"`
	// Metadata
	FromCache bool    `json:"from_cache"`
	CachedAt  *string `json:"cached_at,omitempty"`
}

// Rating holds overall and location ratings for a property.
type Rating struct {
	Overall  *float64 `json:"overall,omitempty"`
	Location *float64 `json:"location,omitempty"`
}

// Price holds per-night and total price details.
type Price struct {
	Currency string      `json:"currency"`
	PerNight PriceDetail `json:"per_night"`
	Total    PriceDetail `json:"total"`
}

// OtherReviewResponse holds a review from an external source.
type OtherReviewResponse struct {
	Source         string                  `json:"source"`
	LogoURL        string                  `json:"logo_url,omitempty"`
	Score          float64                 `json:"score"`
	MaxScore       float64                 `json:"max_score"`
	TotalReviews   int                     `json:"total_reviews"`
	FeaturedReview *FeaturedReviewResponse `json:"featured_review,omitempty"`
}

// FeaturedReviewResponse is a featured user review within an external review.
type FeaturedReviewResponse struct {
	Author  string  `json:"author"`
	Date    string  `json:"date"`
	Score   float64 `json:"score"`
	Comment string  `json:"comment"`
	URL     *string `json:"url,omitempty"`
}

// HotelPriceRangeResponse represents the typical price range for a hotel.
type HotelPriceRangeResponse struct {
	Currency string  `json:"currency"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
}

// HealthAndSafetyCategoryResponse is a category group within health_and_safety.
type HealthAndSafetyCategoryResponse struct {
	Category string                        `json:"category"`
	Items    []HealthAndSafetyItemResponse `json:"items"`
}

// HealthAndSafetyItemResponse is a single health/safety measure.
type HealthAndSafetyItemResponse struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// SustainabilityCategoryResponse is a category group within sustainability.
type SustainabilityCategoryResponse struct {
	Category string                      `json:"category"`
	Items    []SustainabilityItemResponse `json:"items"`
}

// SustainabilityItemResponse is a single sustainability measure.
type SustainabilityItemResponse struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}
