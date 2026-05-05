// Mapping functions for hotel/VR SerpAPI types → domain response types.
// Lives in the serpapi adapter package since these types are defined here.
package serpapi

import (
	"fmt"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// MapImages converts SerpAPI hotel images to domain.Image slice.
func MapImages(serpImages []HotelImage) []domain.Image {
	if serpImages == nil {
		return nil
	}
	imgs := make([]domain.Image, len(serpImages))
	for i, si := range serpImages {
		imgs[i] = domain.Image{
			Thumbnail: si.Thumbnail,
			Original:  si.OriginalImage,
		}
	}
	return imgs
}

// MapNearbyPlaces converts SerpAPI nearby places to domain.NearbyPlace slice.
func MapNearbyPlaces(serpPlaces []HotelNearbyPlace) []domain.NearbyPlace {
	if serpPlaces == nil {
		return nil
	}
	places := make([]domain.NearbyPlace, len(serpPlaces))
	for i, sp := range serpPlaces {
		np := domain.NearbyPlace{
			Name:         sp.Name,
			Description:  sp.Description,
			Rating:       sp.Rating,
			TotalReviews: sp.Reviews,
			ThumbnailURL: sp.Thumbnail,
			MapsURL:      sp.Link,
		}
		if sp.Category != nil {
			np.Category = *sp.Category
		}
		if sp.GPSCoordinates != nil {
			np.GPS = &domain.GPS{
				Lat: sp.GPSCoordinates.Latitude,
				Lng: sp.GPSCoordinates.Longitude,
			}
		}
		if len(sp.Transportations) > 0 {
			np.Transport = make([]domain.Transport, len(sp.Transportations))
			for j, t := range sp.Transportations {
				np.Transport[j] = domain.Transport{
					Type:     t.Type,
					Duration: t.Duration,
				}
			}
		}
		places[i] = np
	}
	return places
}

// MapPriceDetail converts a SerpAPI rate detail to a domain.PriceDetail.
func MapPriceDetail(sd HotelRateDetail) domain.PriceDetail {
	pd := domain.PriceDetail{}
	if sd.ExtractedLowest != nil {
		pd.Amount = *sd.ExtractedLowest
	}
	if sd.ExtractedBeforeTaxesFees != nil {
		pd.BeforeTaxes = sd.ExtractedBeforeTaxesFees
	}
	return pd
}

// MapCapacity converts essential info KVs to a domain.Capacity pointer.
func MapCapacity(essentialInfo []HotelEssentialKV) *domain.Capacity {
	if len(essentialInfo) == 0 {
		return nil
	}
	c := &domain.Capacity{}
	for _, kv := range essentialInfo {
		switch kv.Key {
		case "unit_type":
			c.UnitType = kv.Value
		case "guests":
			if n, err := ParseInt(kv.Value); err == nil {
				c.Guests = &n
			}
		case "bedrooms":
			if n, err := ParseInt(kv.Value); err == nil {
				c.Bedrooms = &n
			}
		case "bathrooms":
			if n, err := ParseInt(kv.Value); err == nil {
				c.Bathrooms = &n
			}
		case "beds":
			if n, err := ParseInt(kv.Value); err == nil {
				c.Beds = &n
			}
		case "area":
			c.Area = kv.Value
		}
	}
	return c
}

// MapRatings converts SerpAPI rating distribution to domain.HotelRatingResponse slice.
func MapRatings(serpRatings []HotelRating) []domain.HotelRatingResponse {
	if serpRatings == nil {
		return nil
	}
	out := make([]domain.HotelRatingResponse, len(serpRatings))
	for i, r := range serpRatings {
		out[i] = domain.HotelRatingResponse{
			Stars: r.Stars,
			Count: r.Count,
		}
	}
	return out
}

// MapReviewsBreakdown converts SerpAPI review breakdown to domain.HotelReviewBreakdownResponse slice.
func MapReviewsBreakdown(serpBreakdown []HotelReviewBreakdown) []domain.HotelReviewBreakdownResponse {
	if serpBreakdown == nil {
		return nil
	}
	out := make([]domain.HotelReviewBreakdownResponse, len(serpBreakdown))
	for i, b := range serpBreakdown {
		out[i] = domain.HotelReviewBreakdownResponse{
			Name:           b.Name,
			Description:    b.Description,
			TotalMentioned: b.TotalMentioned,
			Positive:       b.Positive,
			Negative:       b.Negative,
			Neutral:        b.Neutral,
		}
	}
	return out
}

// ParseInt is a helper to parse integer strings from essential_info values.
func ParseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
