package domain

import (
	"context"
	"testing"
)

// =============================================================================
// Compile-time interface satisfaction checks
// =============================================================================

// testFlightProvider is a mock that satisfies the FlightProvider interface.
// If this doesn't compile, the FlightProvider interface has changed.
type testFlightProvider struct{}

func (p *testFlightProvider) SearchFlights(_ context.Context, _ FlightSearchRequest) (*FlightSearchResponse, error) {
	return nil, nil
}
func (p *testFlightProvider) GetFlightDetails(_ context.Context, _ FlightDetailsRequest) (*FlightDetailsResponse, error) {
	return nil, nil
}

// Compile-time assertion: testFlightProvider satisfies FlightProvider
var _ FlightProvider = (*testFlightProvider)(nil)

// testHotelProvider is a mock that satisfies the HotelProvider interface.
// If this doesn't compile, the HotelProvider interface has changed.
type testHotelProvider struct{}

func (p *testHotelProvider) SearchHotels(_ context.Context, _ HotelSearchRequest) (*HotelSearchResponse, error) {
	return nil, nil
}
func (p *testHotelProvider) GetHotelDetails(_ context.Context, _ HotelDetailsRequest) (*HotelDetailsResponse, error) {
	return nil, nil
}

// Compile-time assertion: testHotelProvider satisfies HotelProvider
var _ HotelProvider = (*testHotelProvider)(nil)

// testCombinedProvider implements BOTH interfaces — the same struct implements
// both FlightProvider and HotelProvider via distinct method names.
type testCombinedProvider struct {
	testFlightProvider
	testHotelProvider
}

// Compile-time assertions: one struct satisfies both interfaces
// (promoted methods from both embedded mock types)
var _ FlightProvider = (*testCombinedProvider)(nil)
var _ HotelProvider = (*testCombinedProvider)(nil)

// =============================================================================
// Smoke test: ensure mock providers work at runtime
// =============================================================================

func TestFlightProvider_MockSatisfiesInterface(t *testing.T) {
	var p FlightProvider = &testFlightProvider{}
	ctx := t.Context()

	resp, err := p.SearchFlights(ctx, FlightSearchRequest{})
	if err != nil {
		t.Errorf("SearchFlights returned unexpected error: %v", err)
	}
	if resp != nil {
		t.Error("SearchFlights expected nil response from mock")
	}

	resp2, err := p.GetFlightDetails(ctx, FlightDetailsRequest{})
	if err != nil {
		t.Errorf("GetFlightDetails returned unexpected error: %v", err)
	}
	if resp2 != nil {
		t.Error("GetFlightDetails expected nil response from mock")
	}
}

func TestHotelProvider_MockSatisfiesInterface(t *testing.T) {
	var p HotelProvider = &testHotelProvider{}
	ctx := t.Context()

	resp, err := p.SearchHotels(ctx, HotelSearchRequest{})
	if err != nil {
		t.Errorf("SearchHotels returned unexpected error: %v", err)
	}
	if resp != nil {
		t.Error("SearchHotels expected nil response from mock")
	}

	resp2, err := p.GetHotelDetails(ctx, HotelDetailsRequest{})
	if err != nil {
		t.Errorf("GetHotelDetails returned unexpected error: %v", err)
	}
	if resp2 != nil {
		t.Error("GetHotelDetails expected nil response from mock")
	}
}

func TestCombinedProvider_SatisfiesBothInterfaces(t *testing.T) {
	// One struct implementing both interfaces via promoted methods
	p := &testCombinedProvider{}

	var flightProv FlightProvider = p
	var hotelProv HotelProvider = p

	_, _ = flightProv.SearchFlights(t.Context(), FlightSearchRequest{})
	_, _ = hotelProv.SearchHotels(t.Context(), HotelSearchRequest{})

	// Both method names are distinct — no collision
}
