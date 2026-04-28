// DTOs que mapean la respuesta de SerpAPI.
// Definidos según el formato JSON que retorna la API.
package serpapi

import "encoding/json"

// =============================================================================
// Response de Búsqueda
// =============================================================================

// serpapiSearchResponse is the top-level SerpAPI search response.
type serpapiSearchResponse struct {
	SearchMetadata   serpapiSearchMetadata `json:"search_metadata"`
	SearchParameters json.RawMessage       `json:"search_parameters"`
	BestFlights      []serpapiFlightGroup  `json:"best_flights,omitempty"`
	OtherFlights     []serpapiFlightGroup  `json:"other_flights,omitempty"`
	PriceInsights    *serpapiPriceInsights `json:"price_insights,omitempty"`
	Airports         []serpapiAirportGroup `json:"airports,omitempty"`
	Error            string                `json:"error,omitempty"`
}

type serpapiSearchMetadata struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	JSONEndpoint     string `json:"json_endpoint"`
	CreatedAt        string `json:"created_at"`
	ProcessedAt      string `json:"processed_at"`
	GoogleFlightsURL string `json:"google_flights_url"`
	RawHTMLFile      string `json:"raw_html_file"`
	PrettifyHTMLFile string `json:"prettify_html_file"`
	TotalTimeTaken   float64 `json:"total_time_taken"`
}

// =============================================================================
// Flight Groups y Flights
// =============================================================================

type serpapiFlightGroup struct {
	Flights        []serpapiFlight       `json:"flights"`
	Layovers       []serpapiLayover      `json:"layovers,omitempty"`
	TotalDuration  int                   `json:"total_duration"`
	CarbonEmissions serpapiCarbonEmissions `json:"carbon_emissions"`
	Price          int                   `json:"price"`
	Type           string                `json:"type"`
	AirlineLogo    string                `json:"airline_logo"`
	Extensions     []string              `json:"extensions,omitempty"`
	DepartureToken string                `json:"departure_token,omitempty"`
	BookingToken   string                `json:"booking_token,omitempty"`
}

type serpapiFlight struct {
	DepartureAirport         serpapiAirportTime `json:"departure_airport"`
	ArrivalAirport           serpapiAirportTime `json:"arrival_airport"`
	Duration                 int                `json:"duration"`
	Airplane                 string             `json:"airplane"`
	Airline                  string             `json:"airline"`
	AirlineLogo              string             `json:"airline_logo"`
	TravelClass              string             `json:"travel_class"`
	FlightNumber             string             `json:"flight_number"`
	Extensions               []string           `json:"extensions"`
	TicketAlsoSoldBy         []string           `json:"ticket_also_sold_by,omitempty"`
	Legroom                  string             `json:"legroom,omitempty"`
	Overnight                bool               `json:"overnight,omitempty"`
	OftenDelayedByOver30Min  bool               `json:"often_delayed_by_over_30_min,omitempty"`
	PlaneAndCrewBy           string             `json:"plane_and_crew_by,omitempty"`
}

type serpapiAirportTime struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Time string `json:"time"`
}

type serpapiLayover struct {
	Duration  int    `json:"duration"`
	Name      string `json:"name"`
	ID        string `json:"id"`
	Overnight bool   `json:"overnight,omitempty"`
}

// =============================================================================
// Emisiones de Carbono
// =============================================================================

type serpapiCarbonEmissions struct {
	ThisFlight         int `json:"this_flight"`
	TypicalForThisRoute int `json:"typical_for_this_route"`
	DifferencePercent  int `json:"difference_percent"`
}

// =============================================================================
// Price Insights
// =============================================================================

type serpapiPriceInsights struct {
	LowestPrice       int       `json:"lowest_price"`
	PriceLevel        string    `json:"price_level"`
	TypicalPriceRange [2]int    `json:"typical_price_range"`
	PriceHistory      [][2]int  `json:"price_history,omitempty"`
}

// =============================================================================
// Aeropuertos
// =============================================================================

type serpapiAirportGroup struct {
	Departure []serpapiAirportDetail `json:"departure"`
	Arrival   []serpapiAirportDetail `json:"arrival"`
}

type serpapiAirportDetail struct {
	Airport     serpapiAirportRef `json:"airport"`
	City        string            `json:"city"`
	Country     string            `json:"country"`
	CountryCode string            `json:"country_code"`
	Image       string            `json:"image"`
	Thumbnail   string            `json:"thumbnail"`
}

type serpapiAirportRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// =============================================================================
// Response de Detalles de Reserva
// =============================================================================

type serpapiBookingResponse struct {
	SearchMetadata   serpapiSearchMetadata `json:"search_metadata"`
	SearchParameters json.RawMessage       `json:"search_parameters"`
	SelectedFlights  []serpapiFlightGroup  `json:"selected_flights"`
	BaggagePrices    *serpapiBaggagePrices `json:"baggage_prices,omitempty"`
	BookingOptions   []serpapiBookingOption `json:"booking_options"`
	Error            string                `json:"error,omitempty"`
}

type serpapiBaggagePrices struct {
	Together  []string `json:"together,omitempty"`
	Departing []string `json:"departing,omitempty"`
	Returning []string `json:"returning,omitempty"`
}

type serpapiBookingOption struct {
	SeparateTickets bool                   `json:"separate_tickets,omitempty"`
	Together        serpapiBookingDetail   `json:"together"`
	Departing       *serpapiBookingDetail  `json:"departing,omitempty"`
	Returning       *serpapiBookingDetail  `json:"returning,omitempty"`
}

type serpapiBookingDetail struct {
	BookWith                string                    `json:"book_with"`
	Airline                 bool                      `json:"airline,omitempty"`
	AirlineLogos            []string                  `json:"airline_logos"`
	MarketedAs              []string                  `json:"marketed_as"`
	Price                   float64                   `json:"price"`
	LocalPrices             []serpapiLocalPrice       `json:"local_prices,omitempty"`
	OptionTitle             string                    `json:"option_title,omitempty"`
	Extensions              []string                  `json:"extensions,omitempty"`
	BaggagePrices           []string                  `json:"baggage_prices,omitempty"`
	BookingRequest          *serpapiBookingRequest    `json:"booking_request,omitempty"`
	BookingPhone            string                    `json:"booking_phone,omitempty"`
	EstimatedPhoneServiceFee float64                  `json:"estimated_phone_service_fee,omitempty"`
}

type serpapiLocalPrice struct {
	Currency string  `json:"currency"`
	Price    float64 `json:"price"`
}

type serpapiBookingRequest struct {
	URL      string `json:"url"`
	PostData string `json:"post_data"`
}
