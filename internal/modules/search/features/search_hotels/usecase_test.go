package search_hotels_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ProacTrip/Backend/internal/modules/search/domain"
	"github.com/ProacTrip/Backend/internal/modules/search/features/search_hotels"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
	"github.com/ProacTrip/Backend/internal/shared/ratelimit"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// =============================================================================
// Mocks
// =============================================================================

type stubCache struct {
	getFn func(ctx context.Context, key string) (string, error)
}

func (s *stubCache) Get(ctx context.Context, key string) (string, error) {
	return s.getFn(ctx, key)
}
func (s *stubCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return nil
}

// stubHotelProvider panics via t.Error if SearchHotels is called — used to
// verify that rate limit guards work without calling the provider.
type stubHotelProvider struct {
	t *testing.T
}

func (s *stubHotelProvider) SearchHotels(ctx context.Context, req domain.HotelSearchRequest) (*domain.HotelSearchResponse, error) {
	s.t.Error("SearchHotels() should NOT be called when rate limited")
	return nil, errors.New("unexpected call")
}

func (s *stubHotelProvider) GetHotelDetails(ctx context.Context, req domain.HotelDetailsRequest) (*domain.HotelDetailsResponse, error) {
	return nil, nil
}

// =============================================================================
// Setup
// =============================================================================

func setupRateLimiter(t *testing.T) (*ratelimit.RateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	cfg := ratelimit.DefaultConfig()
	rl := ratelimit.NewRateLimiter(rdb, cfg)
	return rl, mr
}

func newTestCommand() search_hotels.Command {
	return search_hotels.Command{
		Query:        "Madrid",
		CheckInDate:  "2026-06-15",
		CheckOutDate: "2026-06-20",
		Adults:       2,
		Currency:     new("USD"),
		GL:           new("us"),
		HL:           new("en"),
	}
}

// =============================================================================
// Tests
// =============================================================================

// TestExecute_RateLimitDenied verifies that when ProviderAllow returns
// Allowed=false, Execute returns domain.ErrRateLimitExceeded and does NOT
// call the provider.
func TestExecute_RateLimitDenied(t *testing.T) {
	ctx := t.Context()
	rl, _ := setupRateLimiter(t)

	// Exhaust serpapi quota
	for range 50 {
		result, err := rl.ProviderAllow(ctx, "serpapi")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if !result.Allowed {
			t.Fatal("setup failed: exhausted quota too early")
		}
	}

	provider := &stubHotelProvider{t: t}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil // cache miss
		},
	}

	uc := search_hotels.NewUseCase(search_hotels.UseCaseDeps{
		Provider:    provider,
		Cache:       cache,
		SearchTTL:   15 * time.Minute,
		RateLimiter: rl,
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(ctx, cmd)

	if err == nil {
		t.Error("expected error when rate limited, got nil")
	}
	if !errors.Is(err, domain.ErrRateLimitExceeded) {
		t.Errorf("expected ErrRateLimitExceeded, got: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response when rate limited, got %+v", resp)
	}
}

// TestExecute_HappyPath verifies the full happy path: cache miss → provider
// call → success, with real-looking domain response data.
func TestExecute_HappyPath(t *testing.T) {
	ctx := t.Context()

	provider := &stubHappyHotelProvider{}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil // cache miss
		},
	}

	uc := search_hotels.NewUseCase(search_hotels.UseCaseDeps{
		Provider:    provider,
		Cache:       cache,
		SearchTTL:   15 * time.Minute,
		RateLimiter: nil, // no rate limiter in this test
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(ctx, cmd)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Type != "hotels" {
		t.Errorf("resp.Type = %q, want hotels", resp.Type)
	}
	if resp.ResultsState != "matching" {
		t.Errorf("resp.ResultsState = %q, want matching", resp.ResultsState)
	}
	if len(resp.Properties) != 1 {
		t.Fatalf("len(resp.Properties) = %d, want 1", len(resp.Properties))
	}
	p := resp.Properties[0]
	if p.ID != "abc123" {
		t.Errorf("property ID = %q, want abc123", p.ID)
	}
	if p.Name != "Gran Hotel Madrid" {
		t.Errorf("property Name = %q, want Gran Hotel Madrid", p.Name)
	}
	if p.Type != "hotel" {
		t.Errorf("property Type = %q, want hotel", p.Type)
	}
	if p.Price.Currency != "USD" {
		t.Errorf("price currency = %q, want USD", p.Price.Currency)
	}
	if p.Price.PerNight.Amount != 150.0 {
		t.Errorf("price per night amount = %f, want 150.0", p.Price.PerNight.Amount)
	}
	if p.HotelClass == nil || *p.HotelClass != 4 {
		t.Errorf("hotel class = %v, want 4", p.HotelClass)
	}
	if p.CheckIn != "15:00" {
		t.Errorf("check_in = %q, want 15:00", p.CheckIn)
	}
	if p.CheckOut != "12:00" {
		t.Errorf("check_out = %q, want 12:00", p.CheckOut)
	}
	if p.TotalReviews == nil || *p.TotalReviews != 1234 {
		t.Errorf("total reviews = %v, want 1234", p.TotalReviews)
	}
	if p.GPS.Lat != 40.4168 {
		t.Errorf("GPS lat = %f, want 40.4168", p.GPS.Lat)
	}
	if p.GPS.Lng != -3.7038 {
		t.Errorf("GPS lng = %f, want -3.7038", p.GPS.Lng)
	}
	if len(p.Images) != 1 || p.Images[0].Thumbnail != "https://example.com/thumb.jpg" || p.Images[0].Original != "https://example.com/orig.jpg" {
		t.Errorf("images = %+v, want 1 image with thumbnail+original", p.Images)
	}
	if len(p.Ratings) != 1 || p.Ratings[0].Stars != 5 || p.Ratings[0].Count != 800 {
		t.Errorf("ratings = %+v, want 1 entry stars=5 count=800", p.Ratings)
	}
	if resp.Brands == nil {
		t.Error("brands should not be nil")
	}
	if resp.FromCache {
		t.Error("response should not be marked as cached on cache miss")
	}
}

// stubHappyHotelProvider returns a real-looking domain response for happy-path tests.
type stubHappyHotelProvider struct{}

func (s *stubHappyHotelProvider) SearchHotels(ctx context.Context, req domain.HotelSearchRequest) (*domain.HotelSearchResponse, error) {
	hotelClass := 4
	totalReviews := 1234
	freeCancel := true

	return &domain.HotelSearchResponse{
		Type:         "hotels",
		ResultsState: "matching",
		Properties: []domain.HotelProperty{
			{
				ID:          "abc123",
				Type:        "hotel",
				Name:        "Gran Hotel Madrid",
				Description: "A luxurious hotel in the heart of Madrid",
				BookingURL:  "https://example.com/book",
				GPS:         domain.GPS{Lat: 40.4168, Lng: -3.7038},
				HotelClass:  &hotelClass,
				CheckIn:     "15:00",
				CheckOut:    "12:00",
				Rating: domain.HotelPropertyRating{
					Overall:  new(4.5),
					Location: new(4.8),
				},
				TotalReviews: &totalReviews,
				Price: domain.HotelPrice{
					Currency: "USD",
					PerNight: domain.PriceDetail{
						Amount: 150.0,
					},
				},
				Images: []domain.Image{
					{Thumbnail: "https://example.com/thumb.jpg", Original: "https://example.com/orig.jpg"},
				},
				Amenities:        []string{"WiFi", "Pool"},
				FreeCancellation: &freeCancel,
				Ratings: []domain.HotelRatingResponse{
					{Stars: 5, Count: 800},
				},
			},
		},
		Brands:    []domain.HotelBrand{},
		FromCache: false,
	}, nil
}

func (s *stubHappyHotelProvider) GetHotelDetails(ctx context.Context, req domain.HotelDetailsRequest) (*domain.HotelDetailsResponse, error) {
	return nil, nil
}

// TestExecute_RateLimitError verifies that when ProviderAllow returns
// an error, Execute returns an internal error (5xx).
func TestExecute_RateLimitError(t *testing.T) {
	ctx := t.Context()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	cfg := &ratelimit.RateLimitConfig{
		GlobalPerMinute:        100,
		AuthenticatedPerMinute: 10,
		AnonymousPerMinute:     5,
		Providers:              map[string]ratelimit.ProviderLimit{},
	}
	rl := ratelimit.NewRateLimiter(rdb, cfg)

	provider := &stubHotelProvider{t: t}

	cache := &stubCache{
		getFn: func(ctx context.Context, key string) (string, error) {
			return "", nil
		},
	}

	uc := search_hotels.NewUseCase(search_hotels.UseCaseDeps{
		Provider:    provider,
		Cache:       cache,
		SearchTTL:   15 * time.Minute,
		RateLimiter: rl,
	})

	cmd := newTestCommand()
	resp, err := uc.Execute(ctx, cmd)

	if err == nil {
		t.Fatal("expected error when rate limit service is unavailable, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response on rate limit error, got %+v", resp)
	}

	var prob *serrors.Problem
	if !errors.As(err, &prob) {
		t.Fatalf("expected *serrors.Problem, got %T: %v", err, err)
	}
	if prob.Status < 500 {
		t.Errorf("expected 5xx status for rate limit service failure, got %d", prob.Status)
	}
}
