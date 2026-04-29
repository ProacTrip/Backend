// DTO de respuesta para detalles de hotel.
// Contiene campos específicos de hotel y vacation rental.
package hotel_details

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
	Address          *string           `json:"address,omitempty"`
	DirectionsURL    *string           `json:"directions_url,omitempty"`
	PriceRange       *string           `json:"price_range,omitempty"`
	ExternalReviews  []ExternalReview  `json:"external_reviews,omitempty"`
	HealthAndSafety  *string            `json:"health_and_safety,omitempty"`
	Sustainability   *string            `json:"sustainability,omitempty"`
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

// GPS holds latitude and longitude coordinates.
type GPS struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
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

// PriceDetail holds amount and optional before-taxes value.
type PriceDetail struct {
	Amount      float64  `json:"amount"`
	BeforeTaxes *float64 `json:"before_taxes,omitempty"`
}

// Image holds thumbnail and original image URLs.
type Image struct {
	Thumbnail string `json:"thumbnail"`
	Original  string `json:"original"`
}

// NearbyPlace represents a nearby attraction or POI with transport info.
type NearbyPlace struct {
	Name      string      `json:"name"`
	Transport []Transport `json:"transport,omitempty"`
}

// Transport represents a transport option for a nearby place.
type Transport struct {
	Type     string `json:"type"`
	Duration string `json:"duration"`
}

// ExternalReview holds a review from an external source.
type ExternalReview struct {
	Source string  `json:"source"`
	Rating float64 `json:"rating"`
	Count  int     `json:"count"`
	Link   string  `json:"link,omitempty"`
}

// Capacity holds unit type and capacity info (vacation rentals only).
type Capacity struct {
	UnitType  string `json:"unit_type"`
	Guests    *int   `json:"guests,omitempty"`
	Bedrooms  *int   `json:"bedrooms,omitempty"`
	Bathrooms *int   `json:"bathrooms,omitempty"`
	Beds      *int   `json:"beds,omitempty"`
	Area      string `json:"area,omitempty"`
}
