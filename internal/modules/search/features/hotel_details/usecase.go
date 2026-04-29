// Lógica de negocio para obtener detalles de un hotel.
// Orkesta cache y proveedor externo SerpAPI.
package hotel_details

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/adapters/serpapi"
	"github.com/ProacTrip/Backend/internal/modules/search/domain"
)

// =============================================================================
// Puerto de Cache
// =============================================================================

// Cache is the local port interface for cache operations.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// =============================================================================
// UseCase
// =============================================================================

// UseCase orchestrates hotel details retrieval with caching.
type UseCase struct {
	serpapiAdapter *serpapi.Adapter
	cache          Cache
	detailsTTL     time.Duration
	wg             sync.WaitGroup
}

// UseCaseDeps bundles dependencies for the hotel details use case.
type UseCaseDeps struct {
	SerpapiAdapter *serpapi.Adapter
	Cache          Cache
	DetailsTTL     time.Duration
}

// NewUseCase creates a new hotel details use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		serpapiAdapter: deps.SerpapiAdapter,
		cache:          deps.Cache,
		detailsTTL:     deps.DetailsTTL,
	}
}

// Wait blocks until all fire-and-forget goroutines have completed.
func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute retrieves hotel details with caching.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Validate
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Build SerpAPI params
	adapterParams := cmdToDetailsParams(cmd)

	// 3. Generate cache key
	cacheKey := generateCacheKey(adapterParams)

	// 4. Try cache
	if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != "" {
		var resp Response
		if err := json.Unmarshal([]byte(cached), &resp); err == nil {
			resp.FromCache = true
			cachedAt := time.Now().UTC().Format(time.RFC3339)
			resp.CachedAt = &cachedAt
			return &resp, nil
		}
		slog.WarnContext(ctx, "hotel details cache unmarshal failed, falling through to provider",
			slog.String("key", cacheKey),
			slog.Any("err", err),
		)
	}

	// 5. Cache miss — call SerpAPI
	detail, err := uc.serpapiAdapter.HotelDetails(ctx, adapterParams)
	if err != nil {
		return nil, fmt.Errorf("hotel details: %w", err)
	}

	// 6. Map SerpAPI response to our Response
	resp := mapDetailsResponse(detail, cmd.VacationRentals, cmd.Currency)

	// 7. Save to cache async — fire-and-forget
	bgCtx := context.WithoutCancel(ctx)
	uc.wg.Go(func() {
		data, err := json.Marshal(resp)
		if err != nil {
			slog.ErrorContext(bgCtx, "hotel details cache marshal failed",
				slog.String("key", cacheKey),
				slog.Any("err", err),
			)
			return
		}
		if err := uc.cache.Set(bgCtx, cacheKey, string(data), uc.detailsTTL); err != nil {
			slog.ErrorContext(bgCtx, "hotel details cache set failed",
				slog.String("key", cacheKey),
				slog.Any("err", err),
			)
		}
	})

	return resp, nil
}

// =============================================================================
// Mapeo Command → SerpAPI Params
// =============================================================================

func cmdToDetailsParams(cmd Command) serpapi.HotelDetailsParams {
	return serpapi.HotelDetailsParams{
		PropertyToken:   cmd.ID,
		CheckInDate:     cmd.CheckInDate,
		CheckOutDate:    cmd.CheckOutDate,
		Adults:          cmd.Adults,
		Children:        cmd.Children,
		ChildrenAges:    cmd.ChildrenAges,
		GL:              cmd.GL,
		HL:              cmd.HL,
		Currency:        cmd.Currency,
		VacationRentals: cmd.VacationRentals,
	}
}

// =============================================================================
// Generación de Clave de Cache
// =============================================================================

// generateCacheKey builds a deterministic cache key from ALL params that affect
// the hotel details response. Uses blake3 hash for fixed-size keys.
func generateCacheKey(params serpapi.HotelDetailsParams) string {
	raw, err := json.Marshal(params)
	if err != nil {
		// Fallback: limited key (should never happen).
		return fmt.Sprintf("hotel-detail:fallback:%s:%s:%s:%s",
			params.PropertyToken, params.CheckInDate, params.CheckOutDate, params.Currency)
	}
	return "hotel-detail:" + domain.HashKey(raw)
}

// =============================================================================
// Mapeo SerpAPI PropertyDetail → Feature Response
// =============================================================================

func mapDetailsResponse(detail *serpapi.HotelPropertyDetail, vacationRentals bool, currency string) *Response {
	p := detail.HotelProperty

	resp := &Response{
		ID:          p.PropertyToken,
		Type:        p.Type,
		Name:        p.Name,
		Description: p.Description,
		BookingURL:  p.Link,
		GPS: GPS{
			Lat: p.GPSCoordinates.Latitude,
			Lng: p.GPSCoordinates.Longitude,
		},
		HotelClass: p.HotelClass,
		CheckIn:    p.CheckInTime,
		CheckOut:   p.CheckOutTime,
		Rating: Rating{
			Overall:  p.OverallRating,
			Location: p.LocationRating,
		},
		TotalReviews: p.Reviews,
		Price: Price{
			Currency: currency,
			PerNight: mapPriceDetail(p.RatePerNight),
			Total:    mapPriceDetail(p.TotalRate),
		},
		Images:       mapImages(p.Images),
		Amenities:    p.Amenities,
		NearbyPlaces: mapNearbyPlaces(p.NearbyPlaces),

		// Hotel-only detail fields
		Address:         detail.Address,
		DirectionsURL:   detail.DirectionsURL,
		PriceRange:      detail.PriceRange,
		ExternalReviews: mapExternalReviews(detail.ExternalReviews),
		HealthAndSafety: detail.HealthAndSafety,
		Sustainability:  detail.Sustainability,

		// Hotel-only base fields
		FreeCancellation: p.FreeCancellation,
		SpecialOffer:     p.SpecialOffer,
		EcoCertified:     p.EcoCertified,
	}

	// VR-only
	_ = vacationRentals // type already set from p.Type
	if p.Type == "vacation_rental" {
		resp.ExcludedAmenities = p.ExcludedAmenities
		resp.Capacity = mapCapacity(p.EssentialInfo)
	}

	return resp
}

func mapPriceDetail(sd serpapi.HotelRateDetail) PriceDetail {
	pd := PriceDetail{}
	if sd.ExtractedLowest != nil {
		pd.Amount = *sd.ExtractedLowest
	} else if sd.Lowest != nil {
		pd.Amount = *sd.Lowest
	}
	pd.BeforeTaxes = sd.BeforeTaxesFees
	return pd
}

func mapImages(serpImages []serpapi.HotelImage) []Image {
	if serpImages == nil {
		return nil
	}
	imgs := make([]Image, len(serpImages))
	for i, si := range serpImages {
		imgs[i] = Image{
			Thumbnail: si.Thumbnail,
			Original:  si.OriginalImage,
		}
	}
	return imgs
}

func mapNearbyPlaces(serpPlaces []serpapi.HotelNearbyPlace) []NearbyPlace {
	if serpPlaces == nil {
		return nil
	}
	places := make([]NearbyPlace, len(serpPlaces))
	for i, sp := range serpPlaces {
		np := NearbyPlace{
			Name: sp.Name,
		}
		if len(sp.Transportations) > 0 {
			np.Transport = make([]Transport, len(sp.Transportations))
			for j, t := range sp.Transportations {
				np.Transport[j] = Transport{
					Type:     t.Type,
					Duration: t.Duration,
				}
			}
		}
		places[i] = np
	}
	return places
}

func mapExternalReviews(serpReviews []serpapi.HotelExternalReview) []ExternalReview {
	if serpReviews == nil {
		return nil
	}
	reviews := make([]ExternalReview, len(serpReviews))
	for i, sr := range serpReviews {
		reviews[i] = ExternalReview{
			Source: sr.Source,
			Rating: sr.Rating,
			Count:  sr.Count,
			Link:   sr.Link,
		}
	}
	return reviews
}

func mapCapacity(essentialInfo []serpapi.HotelEssentialKV) *Capacity {
	if len(essentialInfo) == 0 {
		return nil
	}
	c := &Capacity{}
	for _, kv := range essentialInfo {
		switch kv.Key {
		case "unit_type":
			c.UnitType = kv.Value
		case "guests":
			if n, err := parseInt(kv.Value); err == nil {
				c.Guests = &n
			}
		case "bedrooms":
			if n, err := parseInt(kv.Value); err == nil {
				c.Bedrooms = &n
			}
		case "bathrooms":
			if n, err := parseInt(kv.Value); err == nil {
				c.Bathrooms = &n
			}
		case "beds":
			if n, err := parseInt(kv.Value); err == nil {
				c.Beds = &n
			}
		case "area":
			c.Area = kv.Value
		}
	}
	return c
}

// =============================================================================
// Utilidades
// =============================================================================

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
