// Lógica de negocio para búsqueda de hoteles y vacation rentals.
// Orkesta cache y proveedor externo SerpAPI.
package search_hotels

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

// UseCase orchestrates hotel search with caching.
type UseCase struct {
	serpapiAdapter *serpapi.Adapter
	cache          Cache
	searchTTL      time.Duration
	wg             sync.WaitGroup
}

// UseCaseDeps bundles dependencies for the search hotels use case.
type UseCaseDeps struct {
	SerpapiAdapter *serpapi.Adapter
	Cache          Cache
	SearchTTL      time.Duration
}

// NewUseCase creates a new search hotels use case.
func NewUseCase(deps UseCaseDeps) *UseCase {
	return &UseCase{
		serpapiAdapter: deps.SerpapiAdapter,
		cache:          deps.Cache,
		searchTTL:      deps.SearchTTL,
	}
}

// Wait blocks until all fire-and-forget goroutines have completed.
func (uc *UseCase) Wait() {
	uc.wg.Wait()
}

// =============================================================================
// Ejecución Principal
// =============================================================================

// Execute performs the hotel search with caching.
func (uc *UseCase) Execute(ctx context.Context, cmd Command) (*Response, error) {
	// 1. Validate
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. Build SerpAPI params
	adapterParams := cmdToAdapterParams(cmd)

	// 3. Generate cache key (exclude page_token from key — different pages are different requests)
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
		slog.WarnContext(ctx, "hotel search cache unmarshal failed, falling through to provider",
			slog.String("key", cacheKey),
			slog.Any("err", err),
		)
	}

	// 5. Cache miss — call SerpAPI
	serpResp, err := uc.serpapiAdapter.SearchHotels(ctx, adapterParams)
	if err != nil {
		return nil, fmt.Errorf("hotel search: %w", err)
	}

	// 6. Map SerpAPI response to our Response
	resp := mapSearchResponse(serpResp, cmd.VacationRentals, cmd.Currency)

	// 7. Save to cache async — fire-and-forget with WaitGroup tracking
	bgCtx := context.WithoutCancel(ctx)
	uc.wg.Go(func() {
		data, err := json.Marshal(resp)
		if err != nil {
			slog.ErrorContext(bgCtx, "hotel search cache marshal failed",
				slog.String("key", cacheKey),
				slog.Any("err", err),
			)
			return
		}
		if err := uc.cache.Set(bgCtx, cacheKey, string(data), uc.searchTTL); err != nil {
			slog.ErrorContext(bgCtx, "hotel search cache set failed",
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

func cmdToAdapterParams(cmd Command) serpapi.HotelSearchParams {
	return serpapi.HotelSearchParams{
		Query:            cmd.Query,
		GL:               cmd.GL,
		HL:               cmd.HL,
		Currency:         cmd.Currency,
		CheckInDate:      cmd.CheckInDate,
		CheckOutDate:     cmd.CheckOutDate,
		Adults:           cmd.Adults,
		Children:         cmd.Children,
		ChildrenAges:     cmd.ChildrenAges,
		SortBy:           cmd.SortBy,
		MinPrice:         cmd.MinPrice,
		MaxPrice:         cmd.MaxPrice,
		PropertyTypes:    cmd.PropertyTypes,
		Amenities:        cmd.Amenities,
		VacationRentals:  cmd.VacationRentals,
		Rating:           cmd.Rating,
		HotelClasses:     cmd.HotelClasses,
		Brands:           cmd.Brands,
		FreeCancellation: cmd.FreeCancellation,
		SpecialOffers:    cmd.SpecialOffers,
		EcoCertified:     cmd.EcoCertified,
		Bedrooms:         cmd.Bedrooms,
		Bathrooms:        cmd.Bathrooms,
		PageToken:        cmd.PageToken,
	}
}

// =============================================================================
// Generación de Clave de Cache
// =============================================================================

// generateCacheKey builds a deterministic cache key from adapter params.
// Excludes page_token from the hash (different pages are different cache keys).
func generateCacheKey(params serpapi.HotelSearchParams) string {
	// Create a copy without page_token for the cache key
	params.PageToken = ""
	raw, err := json.Marshal(params)
	if err != nil {
		// Fallback: limited key
		return fmt.Sprintf("hotels:fallback:%s:%s:%s:%s",
			params.Query, params.CheckInDate, params.CheckOutDate, params.Currency)
	}
	return "hotels:" + domain.HashKey(raw)
}

// =============================================================================
// Mapeo SerpAPI Response → Feature Response
// =============================================================================

func mapSearchResponse(raw *serpapi.HotelSearchResponse, vacationRentals bool, currency string) *Response {
	resp := &Response{
		ResultsState: "matching",
	}

	// Determine type
	if vacationRentals {
		resp.Type = "vacation_rentals"
	} else {
		resp.Type = "hotels"
	}

	// Map results state
	if raw.SearchInformation.HotelsResultsState == "Non-matching results only" {
		resp.ResultsState = "non_matching_only"
	}

	// Map properties
	resp.Properties = mapProperties(raw.NonMatchingProperties, currency)

	// Map brands
	resp.Brands = mapBrands(raw.Brands)

	return resp
}

func mapProperties(serpProps []serpapi.HotelProperty, currency string) []Property {
	if serpProps == nil {
		return nil
	}
	props := make([]Property, 0, len(serpProps))
	for _, sp := range serpProps {
		props = append(props, mapProperty(sp, currency))
	}
	return props
}

func mapProperty(sp serpapi.HotelProperty, currency string) Property {
	p := Property{
		ID:          sp.PropertyToken,
		Type:        sp.Type,
		Name:        sp.Name,
		Description: sp.Description,
		BookingURL:  sp.Link,
		GPS: GPS{
			Lat: sp.GPSCoordinates.Latitude,
			Lng: sp.GPSCoordinates.Longitude,
		},
		HotelClass: sp.HotelClass,
		CheckIn:    sp.CheckInTime,
		CheckOut:   sp.CheckOutTime,
		Rating: Rating{
			Overall:  sp.OverallRating,
			Location: sp.LocationRating,
		},
		TotalReviews: sp.Reviews,
		Price: Price{
			Currency: currency,
			PerNight: mapPriceDetail(sp.RatePerNight),
			Total:    mapPriceDetail(sp.TotalRate),
		},
		Images:       mapImages(sp.Images),
		Amenities:    sp.Amenities,
		NearbyPlaces: mapNearbyPlaces(sp.NearbyPlaces),
		// Hotel-only
		FreeCancellation: sp.FreeCancellation,
		SpecialOffer:     sp.SpecialOffer,
		EcoCertified:     sp.EcoCertified,
	}

	// VR-only
	if sp.Type == "vacation_rental" {
		p.ExcludedAmenities = sp.ExcludedAmenities
		p.Capacity = mapCapacity(sp.EssentialInfo)
	}

	return p
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

func mapBrands(serpBrands []serpapi.HotelBrand) []Brand {
	if serpBrands == nil {
		return nil
	}
	brands := make([]Brand, len(serpBrands))
	for i, sb := range serpBrands {
		b := Brand{
			ID:   sb.ID,
			Name: sb.Name,
		}
		if len(sb.Chains) > 0 {
			b.Chains = make([]Chain, len(sb.Chains))
			for j, sc := range sb.Chains {
				b.Chains[j] = Chain{
					ID:   sc.ID,
					Name: sc.Name,
				}
			}
		}
		brands[i] = b
	}
	return brands
}

// =============================================================================
// Utilidades
// =============================================================================

// parseInt is a helper to parse integer strings from essential_info values.
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
