// DTO de respuesta para búsqueda de hoteles y vacation rentals.
// Contiene los tipos específicos del feature (Property, Price, Rating, etc).
package search_hotels

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

// Capacity holds unit type and capacity info (vacation rentals only).
type Capacity struct {
	UnitType  string `json:"unit_type"`
	Guests    *int   `json:"guests,omitempty"`
	Bedrooms  *int   `json:"bedrooms,omitempty"`
	Bathrooms *int   `json:"bathrooms,omitempty"`
	Beds      *int   `json:"beds,omitempty"`
	Area      string `json:"area,omitempty"`
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
