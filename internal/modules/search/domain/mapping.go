// Shared response types used across hotel search features.
// Extracted from duplicated definitions in search_hotels and hotel_details.
package domain

// GPS holds latitude and longitude coordinates.
type GPS struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Transport represents a transport option for a nearby place.
type Transport struct {
	Type     string `json:"type"`
	Duration string `json:"duration"`
}

// Image holds thumbnail and original image URLs.
type Image struct {
	Thumbnail string `json:"thumbnail"`
	Original  string `json:"original"`
}

// NearbyPlace represents a nearby attraction or POI with transport info.
type NearbyPlace struct {
	Name         string      `json:"name"`
	Category     string      `json:"category,omitzero"`
	Description  *string     `json:"description,omitzero"`
	Rating       *float64    `json:"rating,omitzero"`
	TotalReviews *int        `json:"total_reviews,omitzero"`
	ThumbnailURL *string     `json:"thumbnail_url,omitzero"`
	MapsURL      *string     `json:"maps_url,omitzero"`
	GPS          *GPS        `json:"gps,omitzero"`
	Transport    []Transport `json:"transport,omitzero"`
}

// PriceDetail holds amount and optional before-taxes value.
type PriceDetail struct {
	Amount      float64  `json:"amount"`
	BeforeTaxes *float64 `json:"before_taxes,omitzero"`
}

// Capacity holds unit type and capacity info (vacation rentals only).
type Capacity struct {
	UnitType  string `json:"unit_type"`
	Guests    *int   `json:"guests,omitzero"`
	Bedrooms  *int   `json:"bedrooms,omitzero"`
	Bathrooms *int   `json:"bathrooms,omitzero"`
	Beds      *int   `json:"beds,omitzero"`
	Area      string `json:"area,omitzero"`
}

// HotelRatingResponse represents a star rating distribution bucket.
type HotelRatingResponse struct {
	Stars int `json:"stars"`
	Count int `json:"count"`
}

// HotelReviewBreakdownResponse represents a review category with sentiment counts.
type HotelReviewBreakdownResponse struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	TotalMentioned int    `json:"total_mentioned"`
	Positive       int    `json:"positive"`
	Negative       int    `json:"negative"`
	Neutral        int    `json:"neutral"`
}
