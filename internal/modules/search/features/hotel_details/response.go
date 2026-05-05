// Type alias — response types live in domain, shared across all features.
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
// Response — type alias to domain response
// =============================================================================

// Response is the hotel details API response.
type Response = domain.HotelDetailsResponse
