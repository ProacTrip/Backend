// Domain entities y tipos de negocio para búsqueda de vuelos.
// Define request, response, y todos los tipos relacionados con vuelos.
package domain

import "time"

const (
	DefaultLimit    = 10
	MaxResultsLimit = 100
)

// =============================================================================
// FlightDetailsRequest
// =============================================================================

// FlightDetailsRequest packs all params required to fetch flight booking details.
type FlightDetailsRequest struct {
	BookingToken string `json:"booking_token"`
	Adults       int    `json:"adults"`
	DepartureID  string `json:"departure"`
	ArrivalID    string `json:"arrival"`
	OutboundDate string `json:"outbound_date"`
	ReturnDate   string `json:"return_date"`
	GL           string `json:"gl,omitzero"`
	HL           string `json:"hl,omitzero"`
	Currency     string `json:"currency,omitzero"`
}

// =============================================================================
// Request de Búsqueda
// =============================================================================

// FlightSearchRequest is the domain representation of a search request
// after mapping from the API command.
type FlightSearchRequest struct {
	TripType                string         `json:"trip_type,omitzero"`
	Departure               string         `json:"departure,omitzero"`
	Arrival                 string         `json:"arrival,omitzero"`
	OutboundDate            string         `json:"outbound_date,omitzero"`
	ReturnDate              string         `json:"return_date,omitzero"`
	Legs                    []MultiCityLeg `json:"legs,omitzero"`
	Adults                  int            `json:"adults,omitzero"`
	Children                int            `json:"children,omitzero"`
	InfantsInSeat           int            `json:"infants_in_seat,omitzero"`
	InfantsOnLap            int            `json:"infants_on_lap,omitzero"`
	TravelClass             string         `json:"travel_class,omitzero"`
	GL                      string         `json:"gl,omitzero"`
	HL                      string         `json:"hl,omitzero"`
	Currency                string         `json:"currency,omitzero"`
	Bags                    int            `json:"bags,omitzero"`
	MaxPrice                *float64       `json:"max_price,omitzero"`
	SortBy                  string         `json:"sort_by,omitzero"`
	Stops                   string         `json:"stops,omitzero"`
	IncludeAirlines         []string       `json:"include_airlines,omitzero"`
	ExcludeAirlines         []string       `json:"exclude_airlines,omitzero"`
	OutboundTimes           *TimeRange     `json:"outbound_times,omitzero"`
	ReturnTimes             *TimeRange     `json:"return_times,omitzero"`
	EmissionsFilter         bool           `json:"emissions_filter,omitzero"`
	LayoverDuration         *LayoverRange  `json:"layover_duration,omitzero"`
	ExcludeConnections      []string       `json:"exclude_connections,omitzero"`
	MaxDurationMinutes      *int           `json:"max_duration_minutes,omitzero"`
	OutboundSelectionToken  string         `json:"outbound_selection_token,omitzero"`
	Cursor                  *string        `json:"cursor,omitzero"`
	Limit                   int            `json:"limit,omitzero"`
}

// =============================================================================
// Tipos Auxiliares
// =============================================================================

// TimeRange filters departure and arrival times.
type TimeRange struct {
	DepartureFrom int  `json:"departure_from"`
	DepartureTo   int  `json:"departure_to"`
	ArrivalFrom   *int `json:"arrival_from,omitzero"`
	ArrivalTo     *int `json:"arrival_to,omitzero"`
}

// LayoverRange filters layover duration between connections.
type LayoverRange struct {
	MinMinutes int `json:"min_minutes"`
	MaxMinutes int `json:"max_minutes"`
}

// MultiCityLeg represents one leg in a multi-city search.
type MultiCityLeg struct {
	Departure string     `json:"departure"`
	Arrival   string     `json:"arrival"`
	Date      string     `json:"date"`
	Times     *TimeRange `json:"times,omitzero"`
}

// =============================================================================
// Response de Búsqueda
// =============================================================================

// PaginationMeta contains cursor-based pagination metadata included in every
// search response.
type PaginationMeta struct {
	NextCursor *string `json:"next_cursor,omitzero"`
	PrevCursor *string `json:"prev_cursor,omitzero"`
	HasNext    bool    `json:"has_next"`
	Limit      int     `json:"limit"`
}

// FlightSearchResponse is the domain search response.
type FlightSearchResponse struct {
	TripType      string         `json:"trip_type"`
	Phase         string         `json:"phase"`
	ResultsState  string         `json:"results_state"`
	BestFlights   []Flight       `json:"best_flights"`
	OtherFlights  []Flight       `json:"other_flights"`
	Airports      []Airport      `json:"airports,omitzero"`
	PriceInsights *PriceInsights `json:"price_insights,omitzero"`
	FromCache     bool           `json:"from_cache"`
	CachedAt      *time.Time     `json:"cached_at,omitzero"`
	Meta          *PaginationMeta `json:"meta,omitzero"`
}

// FlightDetailsResponse is the domain response for booking details.
type FlightDetailsResponse struct {
	Itinerary      FlightItinerary  `json:"itinerary"`
	BookingOptions []BookingOption  `json:"booking_options"`
	FromCache      bool             `json:"from_cache"`
	CachedAt       *time.Time       `json:"cached_at,omitzero"`
}

// FlightItinerary holds the outbound and optional return flight details.
type FlightItinerary struct {
	TripType string        `json:"trip_type"`
	Outbound FlightDetail  `json:"outbound"`
	Return   *FlightDetail `json:"return,omitzero"`
}

// FlightDetail contains legs, layovers, duration, and emissions for one direction.
type FlightDetail struct {
	Legs                 []Leg            `json:"legs"`
	Layovers             []Layover        `json:"layovers,omitzero"`
	TotalDurationMinutes int              `json:"total_duration_minutes"`
	CarbonEmissions      CarbonEmissions  `json:"carbon_emissions"`
}

// =============================================================================
// Vuelo
// =============================================================================

// Flight represents a flight result group (can contain multiple legs + layovers).
type Flight struct {
	DepartureToken      string          `json:"departure_token,omitzero"`
	BookingToken        string          `json:"booking_token,omitzero"`
	Legs                []Leg           `json:"legs"`
	Layovers            []Layover       `json:"layovers,omitzero"`
	TotalDurationMinutes int            `json:"total_duration_minutes"`
	Price               PriceInfo       `json:"price"`
	CarbonEmissions     CarbonEmissions `json:"carbon_emissions"`
	Type                string          `json:"type"`
	AirlineLogoURL      string          `json:"airline_logo_url"`
}

// Leg represents a single flight segment.
type Leg struct {
	Departure      AirportTime     `json:"departure"`
	Arrival        AirportTime     `json:"arrival"`
	DurationMinutes int            `json:"duration_minutes"`
	Aircraft       string          `json:"aircraft"`
	Airline        string          `json:"airline"`
	AirlineCode    string          `json:"airline_code"`
	AirlineLogoURL string          `json:"airline_logo_url"`
	FlightNumber   string          `json:"flight_number"`
	TravelClass    string          `json:"travel_class"`
	Legroom        string          `json:"legroom"`
	LegroomQuality string          `json:"legroom_quality"`
	AlsoSoldBy     []string        `json:"also_sold_by,omitzero"`
	Features       FlightFeatures  `json:"features"`
	Overnight      bool            `json:"overnight"`
	OftenDelayed   bool            `json:"often_delayed"`
	OperatedBy     string          `json:"operated_by,omitzero"`
}

// FlightFeatures contains parsed in-flight amenities.
type FlightFeatures struct {
	WiFi          string   `json:"wifi"`
	PowerOutlets  bool     `json:"power_outlets"`
	USB           bool     `json:"usb"`
	Entertainment string   `json:"entertainment"`
	Raw           []string `json:"raw,omitzero"`
}

// AirportTime represents departure or arrival at an airport.
type AirportTime struct {
	AirportCode string `json:"airport_code"`
	AirportName string `json:"airport_name"`
	City        string `json:"city"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	Datetime    string `json:"datetime"`
}

// Layover represents a connection between flights.
type Layover struct {
	AirportCode     string `json:"airport_code"`
	AirportName     string `json:"airport_name"`
	DurationMinutes int    `json:"duration_minutes"`
	Overnight       bool   `json:"overnight"`
}

// Airport represents an airport in the airports section of the response.
type Airport struct {
	Role         string `json:"role"`
	AirportCode  string `json:"airport_code"`
	AirportName  string `json:"airport_name"`
	City         string `json:"city"`
	Country      string `json:"country"`
	CountryCode  string `json:"country_code"`
	ImageURL     string `json:"image_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// PriceInfo represents a monetary price.
type PriceInfo struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// CarbonEmissions represents carbon footprint data for a flight.
type CarbonEmissions struct {
	ThisFlightGrams    int `json:"this_flight_grams"`
	TypicalRouteGrams  int `json:"typical_route_grams"`
	DifferencePercent  int `json:"difference_percent"`
}

// =============================================================================
// Price Insights
// =============================================================================

// PriceInsights contains pricing meta-information.
type PriceInsights struct {
	LowestPrice  PriceInfo    `json:"lowest_price"`
	PriceLevel   string       `json:"price_level"`
	TypicalRange PriceRange   `json:"typical_range"`
	PriceHistory []PricePoint `json:"price_history,omitzero"`
}

// PriceRange represents a min/max price range.
type PriceRange struct {
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Currency string  `json:"currency"`
}

// PricePoint is a timestamped price observation.
type PricePoint struct {
	Timestamp int64   `json:"timestamp"`
	Price     float64 `json:"price"`
}

// =============================================================================
// Opciones de Reserva
// =============================================================================

// BookingOption represents a way to book a flight.
type BookingOption struct {
	TripType        string         `json:"trip_type"`
	SeparateTickets bool           `json:"separate_tickets"`
	Together        BookingDetail  `json:"together"`
	Departing       *BookingDetail `json:"departing,omitzero"`
	Returning       *BookingDetail `json:"returning,omitzero"`
}

// BookingDetail contains details about a specific booking path.
type BookingDetail struct {
	BookWith             string          `json:"book_with"`
	Airline              bool            `json:"airline"`
	AirlineLogos         []string        `json:"airline_logos,omitzero"`
	MarketedAs           []string        `json:"marketed_as,omitzero"`
	Price                float64         `json:"price"`
	LocalPrices          []LocalPrice    `json:"local_prices,omitzero"`
	OptionTitle          string          `json:"option_title"`
	BaggagePrices        []string        `json:"baggage_prices,omitzero"`
	BookingRequest       *BookingRequest `json:"booking_request,omitzero"`
	BookingPhone         string          `json:"booking_phone,omitzero"`
	EstimatedServiceFee  *float64        `json:"estimated_service_fee,omitzero"`
}

// LocalPrice represents a price in a local currency.
type LocalPrice struct {
	Currency string  `json:"currency"`
	Price    float64 `json:"price"`
}

// BookingRequest contains the URL and POST data to complete a booking.
type BookingRequest struct {
	URL      string `json:"url"`
	PostData string `json:"post_data"`
}
