// DTO de respuesta para búsqueda de hoteles y vacation rentals.
// Contiene los tipos específicos del feature (Property, Price, Rating, etc).
package search_hotels

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
// Response — DTO de salida para search hotels
// =============================================================================

// Response is the hotel search API response.
type Response struct {
	Type         string      `json:"type"`          // "hotels" or "vacation_rentals"
	ResultsState string      `json:"results_state"` // "matching" or "non_matching_only"
	Properties   []Property  `json:"properties"`
	Brands       []Brand     `json:"brands"`
	Pagination   Pagination  `json:"pagination"`
	FromCache    bool        `json:"from_cache"`
	CachedAt     *string     `json:"cached_at,omitempty"`
}

// Property represents a single hotel or vacation rental in search results.
type Property struct {
	ID                string        `json:"id"`                           // property_token
	Type              string        `json:"type"`                         // "hotel" or "vacation_rental"
	Name              string        `json:"name"`
	Description       string        `json:"description,omitempty"`
	BookingURL        string        `json:"booking_url,omitempty"`
	GPS               GPS           `json:"gps"`
	HotelClass        *int          `json:"hotel_class,omitempty"`
	CheckIn           string        `json:"check_in,omitempty"`
	CheckOut          string        `json:"check_out,omitempty"`
	Rating            Rating        `json:"rating"`
	TotalReviews      *int          `json:"total_reviews,omitempty"`
	Price             Price         `json:"price"`
	Images            []Image       `json:"images"`
	Amenities         []string      `json:"amenities"`
	NearbyPlaces      []NearbyPlace `json:"nearby_places"`
	// Hotel-only
	FreeCancellation *bool `json:"free_cancellation,omitempty"`
	SpecialOffer     *bool `json:"special_offer,omitempty"`
	EcoCertified     *bool `json:"eco_certified,omitempty"`
	// VR-only
	ExcludedAmenities []string  `json:"excluded_amenities,omitempty"`
	Capacity          *Capacity `json:"capacity,omitempty"`
	// Star ratings distribution
	Ratings          []HotelRatingResponse          `json:"ratings,omitempty"`
	// Review categories breakdown
	ReviewsBreakdown []HotelReviewBreakdownResponse `json:"reviews_breakdown,omitempty"`
	// Multi-source pricing (VR only)
	Prices           []HotelPriceSourceResponse     `json:"prices,omitempty"`
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

// Brand represents a hotel brand.
type Brand struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Chains []Chain `json:"chains"`
}

// Chain represents a brand chain.
type Chain struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Pagination holds the next page token and whether there are more results.
type Pagination struct {
	NextToken string `json:"next_token"`
	HasMore   bool   `json:"has_more"`
}

// HotelPriceSourceResponse represents pricing from a specific OTA in VR results.
type HotelPriceSourceResponse struct {
	Source       string       `json:"source"`
	Logo         string       `json:"logo,omitempty"`
	NumGuests    *int         `json:"num_guests,omitempty"`
	RatePerNight *PriceDetail `json:"rate_per_night,omitempty"`
}
