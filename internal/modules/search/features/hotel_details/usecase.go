// Lógica de negocio para obtener detalles de un hotel.
// Orquesta cache y proveedor externo SerpAPI.
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
	"github.com/ProacTrip/Backend/internal/modules/search/domain/hotelmapping"
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
		HotelClass: p.ExtractedHotelClass,
		CheckIn:    p.CheckInTime,
		CheckOut:   p.CheckOutTime,
		Rating: Rating{
			Overall:  p.OverallRating,
			Location: p.LocationRating,
		},
		TotalReviews: p.Reviews,
		Price: Price{
			Currency: currency,
			PerNight: hotelmapping.MapPriceDetail(p.RatePerNight),
			Total:    hotelmapping.MapPriceDetail(p.TotalRate),
		},
		Images:       hotelmapping.MapImages(p.Images),
		Amenities:    p.Amenities,
		NearbyPlaces: hotelmapping.MapNearbyPlaces(p.NearbyPlaces),

		// Hotel-only detail fields
		Address:         detail.Address,
		DirectionsURL:   detail.DirectionsURL,
		PriceRange:      mapTypicalPriceRange(detail.TypicalPriceRange, currency),
		ExternalReviews: mapOtherReviews(detail.OtherReviews),
		HealthAndSafety: mapHealthAndSafety(detail.HealthAndSafety),
		Sustainability:  mapSustainability(detail.Sustainability),

		// New fields from embedded HotelProperty
		Ratings:          hotelmapping.MapRatings(p.Ratings),
		ReviewsBreakdown: hotelmapping.MapReviewsBreakdown(p.ReviewsBreakdown),

		// Hotel-only base fields
		FreeCancellation: p.FreeCancellation,
		SpecialOffer:     p.SpecialOffer,
		EcoCertified:     p.EcoCertified,
	}

	// VR-only
	_ = vacationRentals // type already set from p.Type
	if p.Type == "vacation_rental" {
		resp.ExcludedAmenities = p.ExcludedAmenities
		resp.Capacity = hotelmapping.MapCapacity([]serpapi.HotelEssentialKV(p.EssentialInfo))
	}

	return resp
}

// =============================================================================
// Utilidades
// =============================================================================

func mapTypicalPriceRange(tpr *serpapi.HotelTypicalPriceRange, currency string) *HotelPriceRangeResponse {
	if tpr == nil {
		return nil
	}
	return &HotelPriceRangeResponse{
		Currency: currency,
		Min:      tpr.ExtractedLowest,
		Max:      tpr.ExtractedHighest,
	}
}

func mapOtherReviews(serpReviews []serpapi.HotelOtherReview) []OtherReviewResponse {
	if serpReviews == nil {
		return nil
	}
	out := make([]OtherReviewResponse, len(serpReviews))
	for i, sr := range serpReviews {
		or := OtherReviewResponse{
			Source:       sr.Source,
			LogoURL:      sr.SourceIcon,
			TotalReviews: sr.Reviews,
		}
		if sr.SourceRating != nil {
			or.Score = sr.SourceRating.Score
			or.MaxScore = sr.SourceRating.MaxScore
		}
		if sr.UserReview != nil {
			fr := FeaturedReviewResponse{
				Author:  sr.UserReview.Username,
				Date:    sr.UserReview.Date,
				Comment: sr.UserReview.Comment,
			}
			if sr.UserReview.Rating != nil {
				fr.Score = sr.UserReview.Rating.Score
			}
			if sr.UserReview.URL != "" {
				fr.URL = new(sr.UserReview.URL)
			}
			or.FeaturedReview = &fr
		}
		out[i] = or
	}
	return out
}

func mapSustainability(s *serpapi.SustainabilityObject) []SustainabilityCategoryResponse {
	if s == nil {
		return nil
	}
	groups := make([]SustainabilityCategoryResponse, len(s.Groups))
	for i, g := range s.Groups {
		items := make([]SustainabilityItemResponse, len(g.List))
		for j, item := range g.List {
			items[j] = SustainabilityItemResponse{
				Name:      item.Title,
				Available: item.Available,
			}
		}
		groups[i] = SustainabilityCategoryResponse{
			Category: g.Title,
			Items:    items,
		}
	}
	return groups
}

func mapHealthAndSafety(hs *serpapi.HealthAndSafetyObject) []HealthAndSafetyCategoryResponse {
	if hs == nil {
		return nil
	}
	groups := make([]HealthAndSafetyCategoryResponse, len(hs.Groups))
	for i, g := range hs.Groups {
		items := make([]HealthAndSafetyItemResponse, len(g.List))
		for j, item := range g.List {
			items[j] = HealthAndSafetyItemResponse{
				Name:      item.Title,
				Available: item.Available,
			}
		}
		groups[i] = HealthAndSafetyCategoryResponse{
			Category: g.Title,
			Items:    items,
		}
	}
	return groups
}
